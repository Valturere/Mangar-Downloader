//go:build windows

package installation

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

func Busy(scope string) bool {
	return mutexExists(scope, "-Install") || mutexExists(scope, "-Update")
}
func InstallBusy(scope string) bool { return mutexExists(scope, "-Install") }
func installationIdentity(scope string) (browserbridge.Identity, error) {
	return browserbridge.UserIdentity(strings.ToLower(filepath.Clean(scope)))
}
func mutexExists(scope, suffix string) bool {
	id, e := installationIdentity(scope)
	if e != nil {
		return true
	}
	name, e := windows.UTF16PtrFromString(id.Mutex + suffix)
	if e != nil {
		return true
	}
	h, e := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if e == windows.ERROR_FILE_NOT_FOUND {
		return false
	}
	if e != nil {
		return true
	}
	windows.CloseHandle(h)
	return true
}

// Guard duplicates the install gate into NSIS. Windows closes it at parent exit,
// including crash/cancel; the helper can exit before NSIS cleans its plugin dir.
func Guard(parent uint32, scope, ready string) error {
	process, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_DUP_HANDLE, false, parent)
	if e != nil {
		return fmt.Errorf("Установщик уже завершён")
	}
	defer windows.CloseHandle(process)
	id, e := browserbridge.UserIdentity(scope)
	if e != nil {
		return e
	}
	installID, e := installationIdentity(scope)
	if e != nil {
		return e
	}
	installID.Mutex += "-Install"
	lock, first, e := browserbridge.Claim(installID)
	if lock != 0 {
		defer windows.CloseHandle(lock)
	}
	if e != nil || !first {
		return fmt.Errorf("Другой установщик уже работает")
	}
	name, e := windows.UTF16PtrFromString(id.Mutex)
	if e != nil {
		return e
	}
	app, e := windows.OpenMutex(windows.SYNCHRONIZE, false, name)
	if e == nil {
		windows.CloseHandle(app)
		return fmt.Errorf("Закройте Mangar Downloader перед установкой")
	}
	if e != windows.ERROR_FILE_NOT_FOUND {
		return e
	}
	if installedProcessRunning(scope) {
		return fmt.Errorf("Закройте установленный Mangar Downloader перед установкой")
	}
	var remote windows.Handle
	if e = windows.DuplicateHandle(windows.CurrentProcess(), lock, process, &remote, 0, false, windows.DUPLICATE_SAME_ACCESS); e != nil {
		return e
	}
	if e = os.WriteFile(ready, []byte("[guard]\r\nstatus=ready\r\n"), 0600); e != nil {
		var copy windows.Handle
		if windows.DuplicateHandle(process, remote, windows.CurrentProcess(), &copy, 0, false, windows.DUPLICATE_SAME_ACCESS|windows.DUPLICATE_CLOSE_SOURCE) == nil {
			windows.CloseHandle(copy)
		}
		return e
	}
	return nil
}

// An alternative LOCALAPPDATA profile may have a different app mutex while the
// same installed image is still mapped. Do not rename that running executable.
func installedProcessRunning(scope string) bool {
	expected := filepath.Join(scope, "Programs", "Mangar", "YTDLP-GUI-Mangar.exe")
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return true
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) {
			continue
		}
		process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			continue
		}
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		e = windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		windows.CloseHandle(process)
		if e == nil && strings.EqualFold(windows.UTF16ToString(buffer[:size]), expected) {
			return true
		}
	}
	return false
}

func BeginLaunch(scope string) (windows.Handle, error) {
	if mutexExists(scope, "-Update") {
		return 0, fmt.Errorf("Выполняется обновление Mangar Downloader. Повторите запуск после его завершения")
	}
	id, e := installationIdentity(scope)
	if e != nil {
		return 0, e
	}
	id.Mutex += "-Install"
	lock, first, e := browserbridge.Claim(id)
	if e != nil || !first {
		if lock != 0 {
			windows.CloseHandle(lock)
		}
		return 0, fmt.Errorf("Выполняется установка Mangar Downloader. Повторите запуск после её завершения")
	}
	if mutexExists(scope, "-Update") {
		windows.CloseHandle(lock)
		return 0, fmt.Errorf("Выполняется обновление Mangar Downloader")
	}
	if TransactionPending(filepath.Join(scope, "Programs", "Mangar")) {
		windows.CloseHandle(lock)
		return 0, fmt.Errorf("Предыдущая установка Mangar прервана. Запустите установщик повторно для восстановления")
	}
	return lock, nil
}

func TransactionPending(dir string) bool {
	state := transactionState(dir)
	return state != "" && state != "committed"
}
func transactionState(dir string) string {
	info, e := os.Lstat(filepath.Join(dir, "transaction.ini"))
	if os.IsNotExist(e) {
		return ""
	}
	if e != nil || !info.Mode().IsRegular() || info.Size() > 8192 {
		return "unknown"
	}
	data, e := os.ReadFile(filepath.Join(dir, "transaction.ini"))
	if e != nil {
		return "unknown"
	}
	text := string(data)
	if len(data) >= 2 && data[0] == 0xff && data[1] == 0xfe {
		if len(data)%2 != 0 {
			return "unknown"
		}
		runes := make([]uint16, 0, (len(data)-2)/2)
		for i := 2; i+1 < len(data); i += 2 {
			runes = append(runes, uint16(data[i])|uint16(data[i+1])<<8)
		}
		text = windows.UTF16ToString(runes)
	}
	section := ""
	values := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(line[1 : len(line)-1])
			continue
		}
		if section != "transaction" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if ok && (key == "owner" || key == "state") {
			if _, seen := values[key]; seen {
				return "unknown"
			}
			values[key] = strings.TrimSpace(value)
		}
	}
	if values["owner"] != ProductID {
		return "unknown"
	}
	if values["state"] != "pending" && values["state"] != "committed" {
		return "unknown"
	}
	return values["state"]
}

func UpdateLease(scope string) (windows.Handle, error) {
	id, e := installationIdentity(scope)
	if e != nil {
		return 0, e
	}
	id.Mutex += "-Update"
	lock, first, e := browserbridge.Claim(id)
	if e != nil || !first {
		if lock != 0 {
			windows.CloseHandle(lock)
		}
		return 0, fmt.Errorf("Обновление уже выполняется")
	}
	return lock, nil
}
