package main

import (
	"sync"
	"testing"

	"YTDLP-GUI-Mangar/internal/settings"
)

func TestCloseToTrayFalseUsesExitCoordinator(t *testing.T) {
	for _, active := range []int{0, 4} {
		a, tray, _, quits, _, _ := lifecycleFixture(t, active)
		a.settings, _ = settings.Load(t.TempDir())
		if _, err := a.SetCloseToTray(false); err != nil {
			t.Fatal(err)
		}
		prompts := 0
		a.confirmExit = func() (bool, error) { prompts++; return false, nil }
		a.RequestClose()
		if active == 0 {
			if quits.Load() != 1 || tray.stops.Load() != 1 || prompts != 0 {
				t.Fatal("idle X did not exit")
			}
		} else {
			if prompts != 1 || quits.Load() != 0 || a.DownloadSnapshot().Closing || a.hidden {
				t.Fatal("cancelled exit changed work")
			}
			a.confirmExit = func() (bool, error) { prompts++; return true, nil }
			a.RequestClose()
			if prompts != 2 || quits.Load() != 1 || tray.stops.Load() != 1 || !a.DownloadSnapshot().Closing {
				t.Fatal("confirmed X did not shut down")
			}
		}
	}
}
func TestTrayExitIgnoresCloseSetting(t *testing.T) {
	a, tray, _, quits, _, _ := lifecycleFixture(t, 0)
	a.requestExit(true)
	if quits.Load() != 1 || tray.stops.Load() != 1 {
		t.Fatal("tray Exit hidden instead")
	}
}
func TestCloseSettingReloadAndConcurrentClose(t *testing.T) {
	a, _, _, _, _, _ := lifecycleFixture(t, 1)
	dir := t.TempDir()
	a.settings, _ = settings.Load(dir)
	if !a.SettingsSnapshot().CloseToTray {
		t.Fatal("default")
	}
	a.SetCloseToTray(false)
	reloaded, _ := settings.Load(dir)
	if reloaded.Snapshot().CloseToTray {
		t.Fatal("restart")
	}
	a.confirmExit = func() (bool, error) { return false, nil }
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.SetCloseToTray(i%2 == 0)
			a.SettingsSnapshot()
			a.RequestClose()
			a.openWindow()
		}()
	}
	wg.Wait()
	if a.DownloadSnapshot().Closing {
		t.Fatal("cancelled close stopped manager")
	}
}
