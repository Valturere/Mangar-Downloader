package main

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/distribution"
	windowsplatform "YTDLP-GUI-Mangar/internal/platform/windows"
	"errors"
)

type ApplicationInfo struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	StoreURL string `json:"storeURL"`
}

func (a *App) ApplicationInfo() ApplicationInfo {
	return ApplicationInfo{Name: "Mangar Downloader", Version: buildinfo.Current(), StoreURL: distribution.StoreURL()}
}
func (a *App) OpenExtensionStore() error {
	a.mu.Lock()
	closing, ready := a.toolClosing || a.exitPending, a.ctx != nil
	a.mu.Unlock()
	if closing || !ready {
		return errors.New("Приложение ещё не готово или завершает работу")
	}
	return windowsplatform.OpenExtensionStore()
}
