//go:build windows

package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func filesystemMessage(err error) string {
	if errors.Is(err, windows.ERROR_DISK_FULL) || errors.Is(err, windows.ERROR_HANDLE_DISK_FULL) {
		return "Недостаточно места на диске для установки компонентов"
	}
	if errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return "Файл компонента занят другой программой. Повторите позже"
	}
	if errors.Is(err, os.ErrPermission) {
		return "Нет доступа к папке компонентов. Проверьте права и повторите"
	}
	return ""
}

func replaceFile(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
func directorySafe(root, p string) error {
	root = filepath.Clean(root)
	p = filepath.Clean(p)
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("Путь выходит за каталог компонентов")
	}
	// Also inspect existing ancestors of root: a junction must not redirect cleanup.
	for cur := p; ; cur = filepath.Dir(cur) {
		ptr, e := windows.UTF16PtrFromString(cur)
		if e != nil {
			return e
		}
		attr, e := windows.GetFileAttributes(ptr)
		if e == nil && attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return fmt.Errorf("Каталог компонентов содержит reparse point")
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}
func acquireLock(root string) (func(), error) {
	p, err := windows.UTF16PtrFromString(filepath.Join(root, "manager.lock"))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, failure("busy", "Компоненты уже используются другим экземпляром или каталог недоступен", fmt.Sprintf("lock=%v", err))
	}
	return func() { windows.CloseHandle(h) }, nil
}

// Leases share a read handle; GC holds an exclusive handle through RemoveAll.
// The sidecar lives outside immutable versions. Delete-on-close removes it
// atomically on GC completion; crash also releases every kernel lock.
func versionGuard(path string, deleting bool) (func(), error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ)
	share := uint32(windows.FILE_SHARE_READ)
	flags := uint32(windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_OPEN_REPARSE_POINT)
	if deleting {
		access |= windows.GENERIC_WRITE | windows.DELETE
		share = 0
		flags |= windows.FILE_FLAG_DELETE_ON_CLOSE
	}
	h, err := windows.CreateFile(p, access, share, nil, windows.OPEN_ALWAYS, flags, 0)
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}

// A running executable cannot be opened exclusively. Probe every executable
// before cleanup so a locked old FFmpeg does not lose its paired ffprobe.
func versionUnlocked(dir string, t Tool) error {
	var handles []windows.Handle
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for _, exe := range executables(t) {
		p := filepath.Join(dir, exe)
		if _, e := os.Stat(p); os.IsNotExist(e) {
			continue
		}
		ptr, e := windows.UTF16PtrFromString(p)
		if e != nil {
			return e
		}
		h, e := windows.CreateFile(ptr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if e != nil {
			return fmt.Errorf("Старая версия используется или недоступна: %w", e)
		}
		handles = append(handles, h)
	}
	return nil
}
