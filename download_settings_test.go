package main

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/settings"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBrowserDownloadDefaultAndAsk(t *testing.T) {
	for _, ask := range []bool{false, true} {
		for _, media := range []bool{false, true} {
			a := readyBrowserApp(t)
			a.settings, _ = settings.Load(t.TempDir())
			directory := filepath.Join(t.TempDir(), "Mangar")
			a.SetDefaultDownloadDirectory(directory)
			a.SetAskBrowserDownloadDirectory(ask)
			// A manual main-screen choice must never become the automatic destination.
			if !ask {
				a.SetBrowserDirectory(t.TempDir())
			}
			message := browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch"}
			if media {
				message = appCapture()
			}
			if reply := a.browserMessage(context.Background(), message); !reply.OK {
				t.Fatal(reply)
			}
			jobs := a.DownloadSnapshot().Jobs
			if ask {
				if len(jobs) != 0 || len(a.BrowserCaptures()) != 1 {
					t.Fatal("ask bypassed folder flow")
				}
				if err := a.SetBrowserDirectory(directory); err != nil {
					t.Fatal(err)
				}
				jobs = a.DownloadSnapshot().Jobs
			}
			if len(jobs) != 1 || len(a.BrowserCaptures()) != 0 {
				t.Fatal("handoff lost request")
			}
			if _, err := os.Stat(directory); err != nil {
				t.Fatal("default not created", err)
			}
			if a.SettingsSnapshot().DefaultDownloadDirectory != directory {
				t.Fatal("temporary selection changed default")
			}
		}
	}
}

func TestBrowserStartupWaitsForPersistedDownloadSettings(t *testing.T) {
	a := readyBrowserApp(t)
	a.browserReady = make(chan struct{})
	dir := t.TempDir()
	custom := filepath.Join(t.TempDir(), "Сохранено")
	store, _ := settings.Load(dir)
	store.SetDefaultDownloadDirectory(custom)
	result := make(chan browserbridge.Reply, 1)
	go func() {
		result <- a.browserMessage(context.Background(), browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch"})
	}()
	select {
	case <-result:
		t.Fatal("request did not wait for startup")
	case <-time.After(30 * time.Millisecond):
	}
	a.mu.Lock()
	a.settings, _ = settings.Load(dir)
	a.mu.Unlock()
	close(a.browserReady)
	if r := <-result; !r.OK {
		t.Fatal(r)
	}
	if len(a.DownloadSnapshot().Jobs) != 1 || len(a.BrowserCaptures()) != 0 {
		t.Fatal("startup lost request")
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatal("persisted path was not used", err)
	}
}

func TestUnavailableBrowserDefaultRetainsRequestForRecovery(t *testing.T) {
	a := readyBrowserApp(t)
	a.settings, _ = settings.Load(t.TempDir())
	blocked := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocked, []byte("unchanged"), 0600)
	a.SetDefaultDownloadDirectory(blocked)
	if r := a.browserMessage(context.Background(), appCapture()); !r.OK {
		t.Fatal(r)
	}
	pending := a.BrowserCaptures()
	if len(a.DownloadSnapshot().Jobs) != 0 || len(pending) != 1 || !strings.Contains(pending[0].Error, "недоступна") {
		t.Fatal("missing recovery", pending)
	}
	if err := a.SetBrowserDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if len(a.DownloadSnapshot().Jobs) != 1 || len(a.BrowserCaptures()) != 0 || a.SettingsSnapshot().DefaultDownloadDirectory != blocked {
		t.Fatal("recovery changed default or lost request")
	}
}

func TestDownloadSettingsResetAndSessionIsolation(t *testing.T) {
	a := readyBrowserApp(t)
	dir := t.TempDir()
	a.settings, _ = settings.Load(dir)
	custom := t.TempDir()
	a.SetDefaultDownloadDirectory(custom)
	if err := a.SetBrowserDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	reload, _ := settings.Load(dir)
	if reload.Snapshot().DefaultDownloadDirectory != custom {
		t.Fatal("main override persisted")
	}
	snapshot, err := a.ResetDefaultDownloadDirectory()
	if err != nil || snapshot.DefaultDownloadDirectory != settings.DefaultDownloadDirectory() {
		t.Fatal("reset", err)
	}
}
