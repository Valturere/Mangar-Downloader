//go:build windows

package settings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// DefaultDownloadDirectory resolves redirected Downloads without creating it.
func DefaultDownloadDirectory() string {
	base, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DONT_VERIFY)
	if err != nil || base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Downloads")
	}
	return filepath.Join(base, "Mangar")
}

func ValidDownloadDirectory(path string) bool {
	path = strings.ReplaceAll(path, "/", `\`)
	if !filepath.IsAbs(path) || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return false
	}
	volume := filepath.VolumeName(path)
	if strings.HasPrefix(volume, `\\`) {
		if strings.ContainsAny(volume, `<>:"|?*`) {
			return false
		}
		for _, r := range volume {
			if r < 32 {
				return false
			}
		}
	}
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(path, volume), func(r rune) bool { return r == '/' || r == '\\' }) {
		if !filepath.IsLocal(part) || strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
		for _, r := range part {
			if r < 32 {
				return false
			}
		}
	}
	return true
}

// PrepareDownloadDirectory is called only at actual admission, never by Settings.
// A failed probe cannot redirect a job or replace an existing user file.
func PrepareDownloadDirectory(path string) error {
	fail := errors.New("Папка загрузок недоступна. Выберите другую папку")
	if !ValidDownloadDirectory(path) {
		return fail
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return fail
	}
	f, err := os.CreateTemp(path, ".mangar-write-*")
	if err != nil {
		return fail
	}
	defer os.Remove(f.Name())
	_, err = f.Write([]byte{0})
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return fail
	}
	return nil
}

func (s *Store) SetDefaultDownloadDirectory(path string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidDownloadDirectory(path) {
		return s.state, errors.New("Выберите абсолютный путь к папке загрузок")
	}
	state := s.state
	state.DefaultDownloadDirectory = filepath.Clean(path)
	return s.saveLocked(state)
}

func (s *Store) SetAskBrowserDownloadDirectory(value bool) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.AskBrowserDownloadDirectory = value
	return s.saveLocked(state)
}
