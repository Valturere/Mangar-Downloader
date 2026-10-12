//go:build windows

package windowsplatform

import (
	"YTDLP-GUI-Mangar/internal/distribution"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shell32 = windows.NewLazySystemDLL("shell32.dll")
var ole32 = windows.NewLazySystemDLL("ole32.dll")

// OpenExtensionStore can open only the compiled CWS item, never a release-provided URL.
func OpenExtensionStore() error {
	name, err := windows.UTF16PtrFromString(distribution.StoreURL())
	if err != nil {
		return errors.New("Не удалось открыть страницу расширения")
	}
	verb, _ := windows.UTF16PtrFromString("open")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return errors.New("Не удалось подготовить открытие страницы расширения")
	}
	defer ole32.NewProc("CoUninitialize").Call()
	if windows.ShellExecute(0, verb, name, nil, nil, windows.SW_SHOWNORMAL) != nil {
		return errors.New("Не удалось открыть страницу расширения")
	}
	return nil
}

// OpenLogs opens only the application-owned directory, never a UI-supplied path.
func OpenLogs(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("Не удалось определить папку логов")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return errors.New("Не удалось создать папку логов")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.IsDir() {
		return errors.New("Папка логов недоступна")
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("Не удалось открыть папку логов")
	}
	verb, _ := windows.UTF16PtrFromString("open")
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return errors.New("Не удалось подготовить открытие папки логов")
	}
	defer ole32.NewProc("CoUninitialize").Call()
	if err := windows.ShellExecute(0, verb, name, nil, nil, windows.SW_SHOWNORMAL); err != nil {
		return errors.New("Не удалось открыть папку логов")
	}
	return nil
}

// OpenDownloaded accepts only a backend-owned completed MP4 path.
func OpenDownloaded(path string, folder bool) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".mp4") {
		return errors.New("Некорректный путь готового файла")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return errors.New("Готовый файл не найден. Возможно, он перемещён или удалён")
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return errors.New("Не удалось открыть готовый файл")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2)
	if int32(hr) < 0 {
		return errors.New("Не удалось подготовить открытие файла")
	}
	defer ole32.NewProc("CoUninitialize").Call()
	if !folder {
		verb, _ := windows.UTF16PtrFromString("open")
		if err := windows.ShellExecute(0, verb, name, nil, nil, windows.SW_SHOWNORMAL); err != nil {
			return errors.New("Не удалось открыть файл. Проверьте приложение для MP4")
		}
		return nil
	}
	var pidl uintptr
	hr, _, _ = shell32.NewProc("SHParseDisplayName").Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
	runtime.KeepAlive(name)
	if int32(hr) < 0 {
		return errors.New("Не удалось открыть папку готового файла")
	}
	defer ole32.NewProc("CoTaskMemFree").Call(pidl)
	// cidl=0: the absolute file PIDL opens its parent with that file selected.
	hr, _, _ = shell32.NewProc("SHOpenFolderAndSelectItems").Call(pidl, 0, 0, 0)
	if int32(hr) < 0 {
		return errors.New("Не удалось открыть папку готового файла")
	}
	return nil
}
