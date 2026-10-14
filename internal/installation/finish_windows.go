//go:build windows

package installation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"YTDLP-GUI-Mangar/internal/buildinfo"
	"golang.org/x/sys/windows"
)

// The snapshot proves that the requested PID is our actual launching process.
// Open it before acknowledging NSIS, then retain the handle across parent exit:
// PID reuse cannot redirect the wait to a different process.
func finishParent(pid uint32) (windows.Handle, error) {
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return 0, e
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) && entry.ParentProcessID == pid && pid != 0 {
			parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if e != nil {
				return 0, e
			}
			var parentBorn, selfBorn, exited, kernel, user windows.Filetime
			e = windows.GetProcessTimes(parent, &parentBorn, &exited, &kernel, &user)
			if e == nil {
				e = windows.GetProcessTimes(windows.CurrentProcess(), &selfBorn, &exited, &kernel, &user)
			}
			if e != nil || parentBorn.Nanoseconds() >= selfBorn.Nanoseconds() {
				windows.CloseHandle(parent)
				return 0, errors.New("Identity parent установщика не подтверждена")
			}
			return parent, nil
		}
	}
	return 0, errors.New("Неверный parent установщика")
}

func afterInstaller(ctx context.Context, parent windows.Handle, scope string, launch func() error) error {
	for {
		state, e := windows.WaitForSingleObject(parent, 20)
		if e != nil {
			return e
		}
		if state == windows.WAIT_OBJECT_0 {
			break
		}
		if e = ctx.Err(); e != nil {
			return e
		}
	}
	var code uint32
	if e := windows.GetExitCodeProcess(parent, &code); e != nil {
		return e
	}
	if code != 0 {
		return errors.New("Установщик завершился с ошибкой")
	}
	// The process signal and handle cleanup can become visible separately.
	for InstallBusy(scope) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if Busy(scope) || TransactionPending(filepath.Join(scope, "Programs", "Mangar")) {
		return errors.New("Установка не готова к запуску")
	}
	return launch()
}

// LaunchAfterInstall runs from the installed helper, never from NSIS's plugin
// directory. NSIS waits for acknowledgement, not for this process to exit.
func LaunchAfterInstall(pid uint32, ready string) error {
	scope, e := KnownUserScope()
	if e != nil {
		return e
	}
	dir := filepath.Join(scope, "Programs", "Mangar")
	self, e := os.Executable()
	if e != nil || !strings.EqualFold(self, filepath.Join(dir, "mangar-updater.exe")) || !InstallBusy(scope) {
		return errors.New("Запуск разрешён только установленному helper во время установки")
	}
	parent, e := finishParent(pid)
	if e != nil {
		return e
	}
	defer windows.CloseHandle(parent)
	if e = os.WriteFile(ready, []byte("[launch]\r\nstatus=ready\r\n"), 0600); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return afterInstaller(ctx, parent, scope, func() error {
		if e := Verify(dir, buildinfo.Current()); e != nil {
			return e
		}
		command := exec.Command(filepath.Join(dir, "YTDLP-GUI-Mangar.exe"))
		command.Dir = dir
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if e := command.Start(); e != nil {
			return e
		}
		return command.Process.Release()
	})
}
