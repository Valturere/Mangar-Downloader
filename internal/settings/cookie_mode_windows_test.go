package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCookieModeMigrationAndPreservation(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")
	os.WriteFile(file, []byte(`{"version":1,"close_to_tray":false,"future":{"a":42}}`), 0600)
	s, e := Load(dir)
	if e != nil || s.Snapshot().BrowserCookieMode != "auto" || s.Snapshot().CloseToTray {
		t.Fatal(e)
	}
	for _, mode := range []string{"chrome", "edge", "firefox", "off", "auto"} {
		if _, e = s.SetBrowserCookieMode(mode); e != nil {
			t.Fatal(e)
		}
		s, e = Load(dir)
		if e != nil || s.Snapshot().BrowserCookieMode != mode {
			t.Fatal(e)
		}
		s.SetCloseToTray(true)
		data, _ := os.ReadFile(file)
		if !strings.Contains(string(data), `"future"`) {
			t.Fatal("unknown lost")
		}
	}
	if _, e = s.SetBrowserCookieMode("chrome:PRIVATE"); e == nil {
		t.Fatal("accepted profile")
	}
	os.WriteFile(file, []byte(`{"version":1,"browser_cookie_mode":"bad"}`), 0600)
	s, e = Load(dir)
	if e == nil || s.Snapshot().BrowserCookieMode != "auto" {
		t.Fatal("unsafe default")
	}
}
