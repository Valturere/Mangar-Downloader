package main

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/download"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"net/url"
	"time"
)

// This is the only capture DTO exposed to Wails.
type BrowserSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Host      string `json:"host"`
	MediaType string `json:"mediaType"`
}

func (a *App) expireBrowserLocked() {
	order := a.browserOrder[:0]
	for _, id := range a.browserOrder {
		c := a.browserCaptures[id]
		if a.browserPages[id] != "" || (c != nil && time.Since(c.Browser.CapturedAt) < download.CaptureTTL) {
			order = append(order, id)
		} else {
			delete(a.browserCaptures, id)
		}
	}
	a.browserOrder = order
}
func (a *App) BrowserCaptures() []BrowserSummary {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.expireBrowserLocked()
	out := []BrowserSummary{}
	for _, id := range a.browserOrder {
		if page := a.browserPages[id]; page != "" {
			u, _ := url.Parse(page)
			out = append(out, BrowserSummary{ID: id, Title: "Страница из браузера", Host: u.Hostname(), MediaType: "page"})
			continue
		}
		c := a.browserCaptures[id]
		u, _ := url.Parse(c.URL)
		out = append(out, BrowserSummary{ID: id, Title: download.BrowserDisplayTitle(download.Request{URL: c.URL, HTTPContext: &c.HTTP, Browser: &c.Browser}), Host: u.Hostname(), MediaType: c.Browser.MediaType})
	}
	return out
}
func (a *App) browserMessage(ctx context.Context, m browserbridge.Message) browserbridge.Reply {
	// Revalidate page messages at admission, independently of helper/pipe checks.
	if m.Type == "page_url" {
		valid, err := browserbridge.Validate(m, time.Now())
		if err != nil {
			return browserbridge.Failure("invalid")
		}
		m = valid
	}
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-a.browserReady:
	case <-ctx.Done():
		return browserbridge.Failure("closing")
	case <-timer.C:
		return browserbridge.Failure("unavailable")
	}
	a.browserHandoff.Lock()
	defer a.browserHandoff.Unlock()
	a.mu.Lock()
	if a.toolClosing || a.exitPending {
		a.mu.Unlock()
		return browserbridge.Failure("closing")
	}
	if m.Type == "show" {
		a.showLocked()
		a.mu.Unlock()
		return browserbridge.Success()
	}
	a.expireBrowserLocked()
	if len(a.browserCaptures)+len(a.browserPages) >= 32 {
		a.mu.Unlock()
		return browserbridge.Failure("busy")
	}
	var idBytes [16]byte
	if _, e := rand.Read(idBytes[:]); e != nil {
		a.mu.Unlock()
		return browserbridge.Failure("invalid")
	}
	id := hex.EncodeToString(idBytes[:])
	if m.Type == "page_url" {
		a.browserPages[id] = m.URL
	} else {
		a.browserCaptures[id] = m.Capture
	}
	a.browserOrder = append(a.browserOrder, id)
	directory := a.browserDirectory
	if directory == "" {
		a.showLocked()
	}
	ready, appCtx := a.toolDOMReady, a.ctx
	a.mu.Unlock()
	if directory != "" {
		if _, e := a.startBrowserCapture(id, directory); e != nil {
			a.openWindow()
		}
	}
	if ready {
		runtime.EventsEmit(appCtx, "browser:changed")
	}
	return browserbridge.Success()
}
func (a *App) SetBrowserDirectory(directory string) error {
	a.browserHandoff.Lock()
	defer a.browserHandoff.Unlock()
	// This mirrors only the current main-screen selection; no persistence/default change.
	a.mu.Lock()
	if a.toolClosing {
		a.mu.Unlock()
		return errors.New("Приложение завершает работу")
	}
	a.browserDirectory = directory
	a.expireBrowserLocked()
	ids := append([]string(nil), a.browserOrder...)
	a.mu.Unlock()
	for _, id := range ids {
		if _, err := a.startBrowserCapture(id, directory); err != nil {
			return err
		}
	}
	return nil
}
func (a *App) StartBrowserCapture(id, directory string) (download.Snapshot, error) {
	a.browserHandoff.Lock()
	defer a.browserHandoff.Unlock()
	return a.startBrowserCapture(id, directory)
}
func (a *App) startBrowserCapture(id, directory string) (download.Snapshot, error) {
	a.mu.Lock()
	a.expireBrowserLocked()
	c := a.browserCaptures[id]
	page := a.browserPages[id]
	if a.toolClosing || a.exitPending || a.downloadManager == nil || (c == nil && page == "") {
		a.mu.Unlock()
		return download.Snapshot{}, errors.New("Видео недоступно. Отправьте его из расширения снова")
	}
	// Claim before releasing App lock; duplicate calls cannot create duplicate jobs.
	delete(a.browserCaptures, id)
	delete(a.browserPages, id)
	manager := a.downloadManager
	a.mu.Unlock()
	request := download.Request{URL: page, Directory: directory}
	if c != nil {
		request = download.Request{URL: c.URL, Directory: directory, HTTPContext: &c.HTTP, Browser: &c.Browser}
	}
	s, err := manager.Start(request)
	if err != nil {
		a.mu.Lock()
		if !a.toolClosing {
			if c != nil {
				a.browserCaptures[id] = c
			} else {
				a.browserPages[id] = page
			}
			found := false
			for _, v := range a.browserOrder {
				if v == id {
					found = true
				}
			}
			if !found {
				a.browserOrder = append(a.browserOrder, id)
			}
		}
		a.mu.Unlock()
	}
	return s, err
}
