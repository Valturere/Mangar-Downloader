package settings

import (
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDownloadDirectoryDefaultsAndOldSettings(t *testing.T) {
	downloads, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, windows.KF_FLAG_DONT_VERIFY)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"", `{"version":1,"close_to_tray":false,"browser_cookie_mode":"edge","future":42}`} {
		dir := t.TempDir()
		if old != "" {
			if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(old), 0600); err != nil {
				t.Fatal(err)
			}
		}
		s, err := Load(dir)
		if err != nil || s.Snapshot().DefaultDownloadDirectory != filepath.Join(downloads, "Mangar") || s.Snapshot().AskBrowserDownloadDirectory {
			t.Fatal(s.Snapshot(), err)
		}
		if old != "" && (s.Snapshot().CloseToTray || s.Snapshot().BrowserCookieMode != "edge") {
			t.Fatal("migration lost existing values")
		}
		custom := filepath.Join(t.TempDir(), "Ещё нет & 100%")
		if _, err := s.SetDefaultDownloadDirectory(custom); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(custom); !os.IsNotExist(err) {
			t.Fatal("settings created download directory", err)
		}
		if _, err := s.SetAskBrowserDownloadDirectory(true); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(dir)
		if err != nil || reloaded.Snapshot() != s.Snapshot() {
			t.Fatal("restart lost settings", err)
		}
		if old != "" {
			data, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
			var fields map[string]json.RawMessage
			json.Unmarshal(data, &fields)
			if string(fields["future"]) != "42" {
				t.Fatal("future field lost")
			}
		}
	}
}

func TestDownloadDirectoryValidationAndRecovery(t *testing.T) {
	s, _ := Load(t.TempDir())
	before := s.Snapshot()
	for _, path := range []string{"", "relative", `C:relative`, `\rooted`, `C:\bad*`, `C:\a:stream`, `C:\NUL`, `C:\trailing.`, `\\?\C:\device`, "C:\\bad\x00"} {
		if _, err := s.SetDefaultDownloadDirectory(path); err == nil || s.Snapshot() != before {
			t.Fatalf("invalid path accepted: %q", path)
		}
	}
	for _, path := range []string{`D:\Видео\Mangar`, `\\server\share\Mangar`, `E:\`} {
		if !ValidDownloadDirectory(path) {
			t.Fatalf("valid path rejected: %q", path)
		}
	}
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, []byte("preserve"), 0600)
	if err := PrepareDownloadDirectory(file); err == nil {
		t.Fatal("file accepted as directory")
	}
	data, _ := os.ReadFile(file)
	if string(data) != "preserve" {
		t.Fatal("user data changed")
	}
	directory := filepath.Join(t.TempDir(), "new", "Mangar")
	if err := PrepareDownloadDirectory(directory); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 0 {
		t.Fatal("probe leaked", err)
	}
}

func TestDownloadSettingsConcurrentSaveAndFailure(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(dir)
	custom := t.TempDir()
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.SetDefaultDownloadDirectory(custom)
			s.SetAskBrowserDownloadDirectory(i%2 == 0)
			s.Snapshot()
		}()
	}
	wg.Wait()
	reloaded, err := Load(dir)
	if err != nil || reloaded.Snapshot() != s.Snapshot() {
		t.Fatal("disk divergence", err)
	}
	failing, _ := Load(t.TempDir())
	os.Mkdir(failing.path, 0700)
	before := failing.Snapshot()
	if _, err := failing.SetDefaultDownloadDirectory(custom); err == nil || failing.Snapshot() != before {
		t.Fatal("failed save changed default")
	}
}

func TestInvalidPersistedDownloadDirectoryUsesDefault(t *testing.T) {
	for _, value := range []string{`""`, `"relative"`, `null`, `42`} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"close_to_tray":false,"default_download_directory":`+value+`}`), 0600)
		s, err := Load(dir)
		if err != nil || s.Snapshot().DefaultDownloadDirectory != DefaultDownloadDirectory() || s.Snapshot().CloseToTray {
			t.Fatal("invalid folder damaged migration", err)
		}
	}
}
