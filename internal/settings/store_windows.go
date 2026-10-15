//go:build windows

// Package settings owns backend close policy independently of Wails.
package settings

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"unicode/utf8"

	"golang.org/x/sys/windows"
)

type Snapshot struct {
	DefaultDownloadDirectory    string `json:"defaultDownloadDirectory"`
	AskBrowserDownloadDirectory bool   `json:"askBrowserDownloadDirectory"`
	CloseToTray                 bool   `json:"closeToTray"`
	BrowserCookieMode           string `json:"browserCookieMode"`
}
type Store struct {
	mu     sync.Mutex
	path   string
	state  Snapshot
	fields map[string]json.RawMessage
}

func Default() Snapshot {
	return Snapshot{CloseToTray: true, BrowserCookieMode: "auto", DefaultDownloadDirectory: DefaultDownloadDirectory()}
}
func validCookieMode(mode string) bool {
	return mode == "auto" || mode == "chrome" || mode == "edge" || mode == "firefox" || mode == "off"
}

// Load errors use defaults and never include file contents/private paths.
func Load(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "settings.json"), state: Default(), fields: map[string]json.RawMessage{}}
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return s, nil
	}
	bad := errors.New("Настройки недоступны или повреждены; используется сворачивание в трей по умолчанию")
	if err != nil {
		return s, bad
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65537))
	var fields map[string]json.RawMessage
	if err != nil || len(b) > 65536 || !utf8.Valid(b) || json.Unmarshal(b, &fields) != nil || fields == nil {
		return s, bad
	}
	var version int
	if value, ok := fields["version"]; ok && (json.Unmarshal(value, &version) != nil || version != 1) {
		return s, bad
	}
	state := Default()
	if value, ok := fields["default_download_directory"]; ok {
		var path string
		if json.Unmarshal(value, &path) == nil && ValidDownloadDirectory(path) {
			state.DefaultDownloadDirectory = filepath.Clean(path)
		}
	}
	if value, ok := fields["ask_browser_download_directory"]; ok {
		var ask bool
		if json.Unmarshal(value, &ask) == nil {
			state.AskBrowserDownloadDirectory = ask
		}
	}
	if value, ok := fields["browser_cookie_mode"]; ok {
		if json.Unmarshal(value, &state.BrowserCookieMode) != nil || !validCookieMode(state.BrowserCookieMode) {
			return s, bad
		}
	}
	if value, ok := fields["close_to_tray"]; ok {
		var setting *bool
		if json.Unmarshal(value, &setting) != nil || setting == nil {
			return s, bad
		}
		state.CloseToTray = *setting
	}
	s.state, s.fields = state, fields
	return s, nil
}
func (s *Store) Snapshot() Snapshot { s.mu.Lock(); defer s.mu.Unlock(); return s.state }

// Sync + close + same-volume atomic replacement precede the authoritative update.
// Unknown fields survive writes. A malformed file is retained until explicit save.
func (s *Store) SetCloseToTray(value bool) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.state
	state.CloseToTray = value
	return s.saveLocked(state)
}
func (s *Store) SetBrowserCookieMode(value string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validCookieMode(value) {
		return s.state, errors.New("Неизвестный режим cookies браузера")
	}
	state := s.state
	state.BrowserCookieMode = value
	return s.saveLocked(state)
}
func (s *Store) saveLocked(state Snapshot) (Snapshot, error) {
	fail := errors.New("Не удалось сохранить настройку. Проверьте доступ к папке данных и повторите")
	fields := make(map[string]json.RawMessage, len(s.fields)+2)
	for k, v := range s.fields {
		fields[k] = v
	}
	fields["version"] = json.RawMessage("1")
	fields["close_to_tray"], _ = json.Marshal(state.CloseToTray)
	fields["browser_cookie_mode"], _ = json.Marshal(state.BrowserCookieMode)
	fields["default_download_directory"], _ = json.Marshal(state.DefaultDownloadDirectory)
	fields["ask_browser_download_directory"], _ = json.Marshal(state.AskBrowserDownloadDirectory)
	b, err := json.MarshalIndent(fields, "", "  ")
	if err != nil || len(b)+1 > 65536 || os.MkdirAll(filepath.Dir(s.path), 0700) != nil {
		return s.state, fail
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.tmp")
	if err != nil {
		return s.state, fail
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(b, '\n'))
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		from, e1 := windows.UTF16PtrFromString(f.Name())
		to, e2 := windows.UTF16PtrFromString(s.path)
		if e1 != nil || e2 != nil {
			err = fail
		} else {
			err = windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		}
	}
	if err != nil {
		return s.state, fail
	}
	s.state = state
	s.fields = fields
	return s.state, nil
}
