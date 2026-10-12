package main

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const shutdownTimeout = 15 * time.Second
const sessionShutdownTimeout = 4 * time.Second

type trayController interface {
	Available() bool
	Stop() error
}

// RequestClose is the TitleBar policy; native close uses the same path.
func (a *App) RequestClose() {
	a.mu.Lock()
	if a.toolClosing || a.exitPending {
		a.mu.Unlock()
		return
	}
	closeToTray := a.settings == nil || a.settings.Snapshot().CloseToTray
	if closeToTray && a.tray != nil && a.trayError == nil && a.tray.Available() {
		a.hideWindow()
		a.hidden = true
		a.mu.Unlock()
		return
	}
	a.mu.Unlock()
	a.requestExit(true)
}

func (a *App) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	if a.ctx == nil {
		a.ctx = ctx // Native close can precede asynchronous OnStartup.
	}
	ready := a.exitReady
	a.mu.Unlock()
	if ready {
		return false
	}
	// Never wait for a dialog/cleanup on the Wails message loop.
	go a.RequestClose()
	return true
}

func (a *App) openWindow() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending {
		return
	}
	a.showLocked()
}

func (a *App) showLocked() {
	a.showWindow() // Wails restores minimized, retains maximized and requests foreground.
	a.hidden = false
	if a.toolDOMReady {
		runtime.EventsEmit(a.ctx, "window:shown")
	}
}

func (a *App) watchTray(ctx context.Context) {
	timer := time.NewTicker(500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			a.checkTray()
		}
	}
}

// If Shell loses an already hidden icon, make the existing window accessible.
func (a *App) checkTray() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expireBrowserLocked()
	if a.hidden && !a.toolClosing && !a.exitPending && (a.tray == nil || !a.tray.Available()) {
		a.showLocked()
	}
}

func (a *App) requestExit(confirm bool) {
	a.mu.Lock()
	if a.exitPending || a.toolClosing {
		a.mu.Unlock()
		return
	}
	a.exitPending = true
	unfinished := a.downloadManager != nil && !a.downloadManager.StopIfIdle()
	if !unfinished {
		a.beginShutdownLocked()
	}
	if confirm && unfinished {
		// The native adapter restores the owner and opens its dialog in one UI
		// thread operation. A queued Wails Show must not race disabling the owner.
		a.hidden = false
	}
	a.mu.Unlock()
	if confirm && unfinished {
		// Share the gate with folder/TXT dialogs; never stack native dialogs.
		if !a.dialogMu.TryLock() {
			a.mu.Lock()
			a.exitPending = false
			a.showLocked()
			a.mu.Unlock()
			return
		}
		approved, err := a.confirmExit()
		a.dialogMu.Unlock()
		if !approved || err != nil {
			a.mu.Lock()
			a.exitPending = false
			if !a.toolClosing {
				a.showLocked()
			}
			a.mu.Unlock()
			return
		}
	}
	a.shutdown(context.Background())
	a.finishExit()
}

func (a *App) finishExit() {
	a.mu.Lock()
	a.exitReady = true
	a.mu.Unlock()
	a.quitOnce.Do(a.quit)
}

// All accepted Exit paths share this synchronous transition. The manager closes
// admission under its scheduler mutex before any asynchronous cleanup starts.
func (a *App) beginShutdown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.beginShutdownLocked()
}

func (a *App) beginShutdownLocked() {
	a.toolClosing = true
	clear(a.browserCaptures)
	clear(a.browserPages)
	a.browserOrder = nil
	if a.downloadManager != nil {
		a.downloadManager.StopAdmission()
	}
	if a.toolCancel != nil {
		a.toolCancel()
	}
	if a.updateController != nil {
		a.updateController.Cancel()
	}
	if a.lifecycleCancel != nil {
		a.lifecycleCancel()
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.beginShutdown()
	a.shutdownOnce.Do(func() {
		go func() {
			defer close(a.shutdownDone)
			deadline, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			a.mu.Lock()
			manager, tray, updater := a.downloadManager, a.tray, a.updateController
			a.mu.Unlock()
			if updater != nil {
				updater.Close(deadline)
			}
			if a.browserServer != nil {
				a.browserServer.Close()
			}
			if manager != nil {
				if err := manager.Shutdown(deadline); err != nil {
					a.logShutdown("Не удалось завершить загрузки за 15 секунд")
				}
			}
			done := make(chan struct{})
			go func() { a.toolWG.Wait(); close(done) }()
			select {
			case <-done:
			case <-deadline.Done():
				a.logShutdown("Не удалось завершить операцию с компонентами за 15 секунд")
			}
			if tray != nil {
				if err := tray.Stop(); err != nil {
					a.logShutdown(err.Error())
				}
			}
		}()
	})
	select {
	case <-a.shutdownDone:
	case <-ctx.Done():
	}
}

func (a *App) logShutdown(message string) {
	a.mu.Lock()
	ctx, ready := a.ctx, a.toolDOMReady
	a.mu.Unlock()
	if ready {
		runtime.LogError(ctx, message)
	}
}

// WM_ENDSESSION must return promptly, without interactive confirmation. On
// forced termination Windows closes the non-inherited Job Object handles.
func (a *App) systemExit() {
	ctx, cancel := context.WithTimeout(context.Background(), sessionShutdownTimeout)
	defer cancel()
	a.shutdown(ctx)
	a.mu.Lock()
	a.exitReady = true
	a.mu.Unlock()
	go a.quitOnce.Do(a.quit)
}
