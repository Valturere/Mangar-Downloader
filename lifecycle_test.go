package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/download"
	"YTDLP-GUI-Mangar/internal/tools"
)

type fakeTray struct {
	available atomic.Bool
	stops     atomic.Int32
}

func (t *fakeTray) Available() bool { return t.available.Load() }
func (t *fakeTray) Stop() error     { t.available.Store(false); t.stops.Add(1); return nil }

type waitingProvider struct {
	entered chan struct{}
	release chan struct{}
}

func (p waitingProvider) Acquire(ctx context.Context, _ tools.Tool) (*tools.Lease, error) {
	p.entered <- struct{}{}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.release:
		return nil, errors.New("controlled failure")
	}
}

func lifecycleFixture(t *testing.T, n int) (*App, *fakeTray, *atomic.Int32, *atomic.Int32, waitingProvider, string) {
	t.Helper()
	a := NewApp()
	tray := &fakeTray{}
	tray.available.Store(true)
	a.tray = tray
	shows, quits := &atomic.Int32{}, &atomic.Int32{}
	a.showWindow = func() { shows.Add(1) }
	a.hideWindow = func() {}
	a.quit = func() { quits.Add(1) }
	p := waitingProvider{entered: make(chan struct{}, 100), release: make(chan struct{}, 100)}
	dest := t.TempDir()
	a.downloadManager = download.New(download.Options{Paths: apppaths.At(t.TempDir()), Tools: p})
	for range n {
		if _, err := a.StartDownload(download.Request{URL: "https://fixture.invalid/video", Directory: dest}); err != nil {
			t.Fatal(err)
		}
	}
	for range min(n, 3) {
		<-p.entered
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a, tray, shows, quits, p, dest
}

func TestCloseTrayContinuesSchedulerAndRestores(t *testing.T) {
	a, tray, shows, quits, p, _ := lifecycleFixture(t, 4)
	before := a.DownloadSnapshot()
	for range 10 {
		a.RequestClose()
		if !a.hidden || a.DownloadSnapshot().Closing || quits.Load() != 0 {
			t.Fatal("X exited or cancelled")
		}
		a.openWindow()
		if a.hidden {
			t.Fatal("not restored")
		}
	}
	a.RequestClose()
	p.release <- struct{}{}
	<-p.entered // fourth queued job entered while hidden
	a.openWindow()
	s := a.DownloadSnapshot()
	if s.Jobs[3].State != "preparing" || s.Jobs[0].ID != before.Jobs[0].ID || shows.Load() != 11 || tray.stops.Load() != 0 {
		t.Fatal(s)
	}
}

func TestTrayLostWhileHiddenShowsWindow(t *testing.T) {
	a, tray, shows, quits, _, _ := lifecycleFixture(t, 1)
	a.RequestClose()
	tray.available.Store(false)
	a.checkTray()
	if a.hidden || shows.Load() != 1 || quits.Load() != 0 || a.DownloadSnapshot().Closing {
		t.Fatal("lost tray left invisible process or cancelled jobs")
	}
}

func TestExitConfirmCancelThenConfirm(t *testing.T) {
	for _, count := range []int{1, 3, 4, 8} {
		t.Run(string(rune('a'+count)), func(t *testing.T) {
			a, tray, _, quits, _, dest := lifecycleFixture(t, count)
			before := a.DownloadSnapshot()
			a.confirmExit = func() (bool, error) { return false, nil }
			a.requestExit(true)
			if a.toolClosing || a.DownloadSnapshot().Closing || tray.stops.Load() != 0 || quits.Load() != 0 {
				t.Fatal("Cancel transitioned shutdown")
			}
			for i, j := range a.DownloadSnapshot().Jobs {
				if j.State != before.Jobs[i].State {
					t.Fatal("Cancel mutated job")
				}
			}
			a.confirmExit = func() (bool, error) { return true, nil }
			a.requestExit(true)
			if !a.exitReady || !a.toolClosing || !a.DownloadSnapshot().Closing || a.DownloadSnapshot().Active != 0 || tray.stops.Load() != 1 || quits.Load() != 1 {
				t.Fatal("confirmed Exit did not clean up")
			}
			if _, err := a.StartDownload(download.Request{URL: "https://fixture.invalid/v", Directory: dest}); err == nil {
				t.Fatal("enqueue after exit")
			}
			if _, err := a.ImportTXT(filepath.Join(dest, "missing.txt"), dest); err == nil {
				t.Fatal("import after exit")
			}
			if _, err := a.DownloadAction(before.Jobs[0].ID, "retry"); err == nil {
				t.Fatal("retry after exit")
			}
			if _, err := a.ChooseDirectory(); err == nil {
				t.Fatal("dialog after exit")
			}
			if a.beforeClose(context.Background()) {
				t.Fatal("approved Quit blocked")
			}
		})
	}
}

func TestUnavailableTrayUsesSharedConfirmation(t *testing.T) {
	for _, mode := range []string{"nil", "init failure", "lost"} {
		t.Run(mode, func(t *testing.T) {
			a, tray, _, quits, _, _ := lifecycleFixture(t, 1)
			switch mode {
			case "nil":
				a.tray = nil
			case "init failure":
				a.trayError = errors.New("init failure")
			case "lost":
				tray.available.Store(false)
			}
			var confirmations int
			a.confirmExit = func() (bool, error) { confirmations++; return false, nil }
			a.RequestClose()
			if a.hidden || a.toolClosing || confirmations != 1 || quits.Load() != 0 {
				t.Fatal("unsafe fallback")
			}
			a.confirmExit = func() (bool, error) { confirmations++; return true, nil }
			a.RequestClose()
			if quits.Load() != 1 || confirmations != 2 {
				t.Fatal("fallback skipped exit")
			}
		})
	}
}

func TestExitEmptyAndTerminalRowsNoConfirmation(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		a, tray, _, quits, _, dest := lifecycleFixture(t, 0)
		if terminal {
			path := filepath.Join(dest, "invalid.txt")
			os.WriteFile(path, []byte("invalid"), 0600)
			if _, err := a.ImportTXT(path, dest); err != nil {
				t.Fatal(err)
			}
		}
		a.confirmExit = func() (bool, error) { t.Error("unexpected confirmation"); return false, nil }
		a.requestExit(true)
		if tray.stops.Load() != 1 || quits.Load() != 1 {
			t.Fatal("idle Exit")
		}
	}
}

func TestExitReentrancyAndCallbacks(t *testing.T) {
	a, tray, shows, quits, _, _ := lifecycleFixture(t, 4)
	entered, release := make(chan struct{}), make(chan struct{})
	var confirmations atomic.Int32
	a.confirmExit = func() (bool, error) { confirmations.Add(1); close(entered); <-release; return true, nil }
	done := make(chan struct{})
	go func() { a.requestExit(true); close(done) }()
	<-entered
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); a.requestExit(true); a.RequestClose(); a.openWindow(); a.checkTray() }()
	}
	wg.Wait()
	if confirmations.Load() != 1 || shows.Load() != 0 || a.DownloadSnapshot().Closing {
		t.Fatal("reentrant confirmation")
	}
	close(release)
	<-done
	a.RequestClose()
	a.openWindow()
	a.requestExit(true)
	a.shutdown(context.Background())
	a.finishExit()
	if confirmations.Load() != 1 || shows.Load() != 0 || quits.Load() != 1 || tray.stops.Load() != 1 {
		t.Fatal("repeated cleanup/callback")
	}
}

func TestSessionExitSkipsConfirmation(t *testing.T) {
	a, tray, _, quits, _, _ := lifecycleFixture(t, 4)
	quit := make(chan struct{}, 1)
	a.quit = func() { quits.Add(1); quit <- struct{}{} }
	a.confirmExit = func() (bool, error) { t.Error("session confirmation"); return false, nil }
	a.systemExit()
	<-quit
	if !a.exitReady || a.DownloadSnapshot().Active != 0 || tray.stops.Load() != 1 {
		t.Fatal("session shutdown")
	}
}

func TestExitCancelPreservesToolOperationThenConfirmAwaitsIt(t *testing.T) {
	a, _, _, _, _, _ := lifecycleFixture(t, 1)
	a.toolContext, a.toolCancel = context.WithCancel(context.Background())
	entered := make(chan struct{})
	a.toolManager = tools.New(tools.Options{Paths: apppaths.At(t.TempDir()), Source: blockedToolSource{entered: entered}})
	a.toolErrorEvent = func(string) {}
	if err := a.ComponentsAction("install"); err != nil {
		t.Fatal(err)
	}
	<-entered
	a.confirmExit = func() (bool, error) { return false, nil }
	a.requestExit(true)
	if a.toolContext.Err() != nil || !a.toolManager.Snapshot().Busy {
		t.Fatal("Cancel interrupted tool operation")
	}
	a.confirmExit = func() (bool, error) { return true, nil }
	a.requestExit(true)
	if a.toolContext.Err() == nil || a.toolManager.Snapshot().Busy {
		t.Fatal("Exit did not await tool operation")
	}
}

func TestConfirmationErrorAndExistingDialogDoNotCancel(t *testing.T) {
	a, _, _, quits, _, _ := lifecycleFixture(t, 1)
	a.confirmExit = func() (bool, error) { return false, errors.New("native failure") }
	a.requestExit(true)
	if a.toolClosing || a.DownloadSnapshot().Closing || quits.Load() != 0 {
		t.Fatal("dialog error cancelled jobs")
	}
	a.dialogMu.Lock()
	a.confirmExit = func() (bool, error) { t.Error("stacked native dialog"); return true, nil }
	a.requestExit(true)
	a.dialogMu.Unlock()
	if a.toolClosing || a.exitPending || a.DownloadSnapshot().Closing {
		t.Fatal("picker conflict changed queue")
	}
}

func TestSessionExitRacesConfirmationOnce(t *testing.T) {
	a, tray, _, quits, _, _ := lifecycleFixture(t, 4)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	a.confirmExit = func() (bool, error) { close(entered); <-release; return true, nil }
	go func() { a.requestExit(true); close(done) }()
	<-entered
	a.systemExit()
	close(release)
	<-done
	a.finishExit()
	if tray.stops.Load() != 1 || quits.Load() != 1 {
		t.Fatal("session and user Exit duplicated cleanup/Quit")
	}
}

func TestNativeCloseBeforeStartupDoesNotLeaveInvisibleApp(t *testing.T) {
	a := NewApp()
	quit := make(chan struct{})
	a.quit = func() { close(quit) }
	if !a.beforeClose(context.Background()) {
		t.Fatal("native close skipped coordinator")
	}
	<-quit
	a.startup(context.Background())
	if a.downloadManager != nil || a.toolManager != nil || !a.exitReady || a.beforeClose(context.Background()) {
		t.Fatal("early Exit admitted late startup")
	}
}
