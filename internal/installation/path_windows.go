//go:build windows

package installation

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
)

func SafeDirectory(dir string) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir || len(dir) > 220 {
		return errors.New("Недопустимый каталог установки")
	}
	for current := dir; ; current = filepath.Dir(current) {
		p, e := windows.UTF16PtrFromString(current)
		if e != nil {
			return errors.New("Недопустимый каталог установки")
		}
		attr, e := windows.GetFileAttributes(p)
		if e == nil && attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("Установка через junction/symlink запрещена")
		}
		if e != nil && e != windows.ERROR_FILE_NOT_FOUND && e != windows.ERROR_PATH_NOT_FOUND {
			return errors.New("Каталог установки недоступен")
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return nil
}
