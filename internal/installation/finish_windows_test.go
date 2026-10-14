//go:build windows

package installation

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestFinishWaitsForInstallerExitAndLaunchesOnce(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failed-installer"}[fail], func(t *testing.T) {
			scope := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestFinishParent$")
			command.Env = append(os.Environ(), "MANGAR_FINISH_PARENT=1")
			input, e := command.StdinPipe()
			if e != nil {
				t.Fatal(e)
			}
			if e = command.Start(); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { input.Close(); command.Process.Kill() })
			if e = Guard(uint32(command.Process.Pid), scope, filepath.Join(t.TempDir(), "ready.ini")); e != nil {
				t.Fatal(e)
			}
			parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
			if e != nil {
				t.Fatal(e)
			}
			defer windows.CloseHandle(parent)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var launches atomic.Int32
			done := make(chan error, 1)
			go func() {
				done <- afterInstaller(ctx, parent, scope, func() error {
					gate, e := BeginLaunch(scope)
					if e != nil {
						return e
					}
					windows.CloseHandle(gate)
					launches.Add(1)
					return nil
				})
			}()
			select {
			case e := <-done:
				t.Fatalf("launched before Finish: %v", e)
			case <-time.After(100 * time.Millisecond):
			}
			if gate, e := BeginLaunch(scope); e == nil {
				windows.CloseHandle(gate)
				t.Fatal("live installer admitted GUI")
			}
			if fail {
				input.Write([]byte{1})
			} else {
				input.Write([]byte{0})
			}
			input.Close()
			command.Wait()
			e = <-done
			if fail {
				if e == nil || launches.Load() != 0 {
					t.Fatal("failed installer launched app", e)
				}
			} else if e != nil || launches.Load() != 1 {
				t.Fatal("Finish must launch exactly once after guard release", e, launches.Load())
			}
		})
	}
}

func TestFinishParent(t *testing.T) {
	if os.Getenv("MANGAR_FINISH_PARENT") != "1" {
		return
	}
	var code [1]byte
	os.Stdin.Read(code[:])
	os.Exit(int(code[0]))
}

func TestFinishRejectsUnrelatedParentAndTimeout(t *testing.T) {
	parent, e := finishParent(uint32(os.Getppid()))
	if e != nil {
		t.Fatal("actual launching parent rejected", e)
	}
	windows.CloseHandle(parent)
	if h, e := finishParent(uint32(os.Getpid())); e == nil {
		windows.CloseHandle(h)
		t.Fatal("own PID accepted as parent")
	}
	h, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(os.Getpid()))
	if e != nil {
		t.Fatal(e)
	}
	defer windows.CloseHandle(h)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if e = afterInstaller(ctx, h, t.TempDir(), func() error { t.Fatal("timeout launched GUI"); return nil }); e == nil {
		t.Fatal("timeout ignored")
	}
}

func TestNSISFinishUsesAcknowledgedHelper(t *testing.T) {
	data, e := os.ReadFile("../../build/windows/installer/project.nsi")
	if e != nil {
		t.Fatal(e)
	}
	text := string(data)
	start := strings.Index(text, "Function LaunchApp\n")
	if start < 0 {
		start = strings.Index(text, "Function LaunchApp\r\n")
	}
	if start < 0 {
		t.Fatal("LaunchApp missing")
	}
	end := strings.Index(text[start:], "FunctionEnd")
	if end < 0 {
		t.Fatal("LaunchApp end missing")
	}
	finish := text[start : start+end]
	if strings.Contains(finish, `Exec '"$INSTDIR\YTDLP-GUI-Mangar.exe"'`) || !strings.Contains(finish, "--launch-after-install $ParentPID") || !strings.Contains(finish, `$GuardState == "ready"`) {
		t.Fatal("Finish bypasses parent/guard handshake")
	}
	if strings.Contains(text, "MUI_PAGE_DIRECTORY") {
		t.Fatal("fixed per-user installation changed")
	}
}
