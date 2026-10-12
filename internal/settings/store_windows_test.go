//go:build windows

package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestDefaultsPersistenceAndForwardFields(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Данные & 100% 日本語")
	s, err := Load(dir)
	if err != nil || !s.Snapshot().CloseToTray {
		t.Fatal(err, s.Snapshot())
	}
	for _, value := range []bool{false, true} {
		if _, err = s.SetCloseToTray(value); err != nil {
			t.Fatal(err)
		}
		s, err = Load(dir)
		if err != nil || s.Snapshot().CloseToTray != value {
			t.Fatal(err, s.Snapshot())
		}
	}
	path := filepath.Join(dir, "settings.json")
	os.WriteFile(path, []byte(`{"version":1,"future":{"enabled":true}}`), 0600)
	s, err = Load(dir)
	if err != nil || !s.Snapshot().CloseToTray {
		t.Fatal(err)
	}
	s.SetCloseToTray(false)
	b, _ := os.ReadFile(path)
	if !bytes.Contains(b, []byte(`"future"`)) {
		t.Fatal(string(b))
	}
}

func TestCorruptSettingsSafeDefault(t *testing.T) {
	for _, data := range []string{"{broken", "null", `{"version":2}`, `{"close_to_tray":null}`, `{"close_to_tray":"false"}`, "{\"future\":\"\xff\",\"close_to_tray\":false}"} {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0600)
		s, err := Load(dir)
		if err == nil || !s.Snapshot().CloseToTray {
			t.Fatal(data, err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "settings.json"))
		if string(b) != data {
			t.Fatal("load overwrote corrupt file")
		}
	}
}
func TestAtomicSaveFailureKeepsAuthoritativeValue(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(dir)
	// A destination directory deterministically makes atomic replace fail.
	os.Mkdir(filepath.Join(dir, "settings.json"), 0700)
	if _, err := s.SetCloseToTray(false); err == nil || !s.Snapshot().CloseToTray {
		t.Fatal("failed save changed policy")
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".settings-*.tmp"))
	if len(files) != 0 {
		t.Fatal("temp leak")
	}
}
func TestConcurrentSettingsUpdates(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(dir)
	var wg sync.WaitGroup
	for i := range 30 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.SetCloseToTray(i%2 == 0); err != nil {
				t.Error(err)
			}
			s.Snapshot()
		}()
	}
	wg.Wait()
	reloaded, err := Load(dir)
	if err != nil || reloaded.Snapshot() != s.Snapshot() {
		t.Fatal("disk/memory divergence", err)
	}
}

func TestOversizedSavePreservesReadableSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	// Readable compact JSON may exceed the limit after preserving fields and indenting.
	data := []byte(`{"future":"` + strings.Repeat("x", 65500) + `"}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetCloseToTray(false); err == nil || !s.Snapshot().CloseToTray {
		t.Fatal("oversized save changed policy")
	}
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, data) {
		t.Fatal("oversized save changed readable file", err)
	}
}
