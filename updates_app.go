package main

import (
	"YTDLP-GUI-Mangar/internal/updates"
	"context"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"time"
)

func (a *App) UpdateSnapshot() updates.Snapshot {
	a.mu.Lock()
	a.updateRevision++
	revision := a.updateRevision
	c, installing, notice := a.updateController, a.updateInstalling, a.updateError
	a.mu.Unlock()
	if c == nil {
		return updates.Snapshot{Revision: revision, Phase: "idle", Message: "Обновления пока недоступны"}
	}
	state := c.Snapshot()
	state.Revision = revision
	if state.Error == "" {
		state.Error = notice
	}
	if installing {
		state.Busy = true
		if state.Phase == "idle" || state.Phase == "" {
			state.Phase = "installing"
		}
	}
	return state
}
func (a *App) updateAccess() (*updates.Controller, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending || a.updateInstalling || a.updateController == nil {
		return nil, errors.New("Обновления недоступны. Дождитесь завершения операции")
	}
	a.updateError = ""
	return a.updateController, nil
}
func updateBindingError(c *updates.Controller, e error) error {
	if e == nil {
		return nil
	}
	s := c.Snapshot()
	if s.Error != "" {
		return errors.New(s.Error)
	}
	return errors.New("Не удалось выполнить операцию обновления")
}

func (a *App) CheckApplicationUpdate() error {
	c, e := a.updateAccess()
	if e != nil {
		return e
	}
	return updateBindingError(c, c.Check(false))
}
func (a *App) CancelApplicationUpdate() {
	a.mu.Lock()
	c := a.updateController
	closing := a.toolClosing
	a.mu.Unlock()
	if c != nil && !closing {
		c.Cancel()
	}
}

func (a *App) InstallApplicationUpdate() (err error) {
	a.mu.Lock()
	c, manager := a.updateController, a.downloadManager
	if c == nil || manager == nil || a.toolClosing || a.exitPending || a.updateInstalling || a.toolPending || (a.toolManager != nil && a.toolManager.Snapshot().Busy) || !manager.IdleForUpdate() {
		a.updateError = "Дождитесь завершения очереди и операций с компонентами перед обновлением"
		a.mu.Unlock()
		return errors.New("Дождитесь завершения очереди и операций с компонентами перед обновлением")
	}
	a.updateInstalling = true
	a.updateError = ""
	a.mu.Unlock()
	succeeded := false
	defer func() {
		if !succeeded {
			manager.ResumeAfterUpdateFailure()
			c.DiscardPrepared()
		}
		a.mu.Lock()
		a.updateInstalling = false
		if err != nil {
			a.updateError = err.Error()
		}
		ready := a.toolDOMReady && !a.toolClosing
		ctx := a.ctx
		a.mu.Unlock()
		if ready {
			runtime.EventsEmit(ctx, "updates:changed", a.UpdateSnapshot())
		}
	}()
	plan, e := c.Prepare()
	if e != nil {
		return updateBindingError(c, e)
	}
	// Admission may have changed while the installer downloaded. Freeze checks
	// queued/preparing/downloading/merging/finalization atomically without cancel.
	a.mu.Lock()
	if a.toolClosing || a.exitPending || a.toolPending || !manager.FreezeForUpdate() {
		a.mu.Unlock()
		return errors.New("Очередь занята. Завершите загрузки и повторите обновление")
	}
	handshake, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	e = updates.LaunchPrepared(handshake, plan)
	cancel()
	if e != nil {
		a.mu.Unlock()
		return errors.New("Установщик не запущен. Приложение продолжает работу; повторите обновление")
	}
	a.exitPending = true
	a.beginShutdownLocked()
	a.mu.Unlock()
	succeeded = true
	a.shutdown(context.Background())
	a.finishExit()
	return nil
}
