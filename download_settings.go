package main

import (
	"YTDLP-GUI-Mangar/internal/settings"
	"errors"
)

func (a *App) SetDefaultDownloadDirectory(path string) (settings.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending || a.settings == nil {
		return settings.Snapshot{}, errors.New("Настройки недоступны. Повторите позже")
	}
	return a.settings.SetDefaultDownloadDirectory(path)
}

func (a *App) ResetDefaultDownloadDirectory() (settings.Snapshot, error) {
	return a.SetDefaultDownloadDirectory(settings.DefaultDownloadDirectory())
}

func (a *App) SetAskBrowserDownloadDirectory(value bool) (settings.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending || a.settings == nil {
		return settings.Snapshot{}, errors.New("Настройки недоступны. Повторите позже")
	}
	snapshot, err := a.settings.SetAskBrowserDownloadDirectory(value)
	if err == nil {
		a.browserDirectory = ""
	}
	return snapshot, err
}
