package main

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/download"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func appCapture() browserbridge.Message {
	return browserbridge.Message{Version: 1, Type: "media_capture", Capture: &browserbridge.Capture{URL: "https://cdn.test/index.m3u8?token=URL_SECRET", TabID: 1, FrameID: 3, HTTP: download.HTTPContext{Captured: true, Headers: map[string]string{"X-Playback": "HEADER_SECRET"}}, Browser: download.BrowserContext{PageTitle: "Страница https://cdn.test/?token=URL_SECRET COOKIE_SECRET HEADER_SECRET", MediaType: "hls", CapturedAt: time.Now(), Cookies: []download.Cookie{{Name: "s", Value: "COOKIE_SECRET", Domain: "cdn.test", Path: "/", HostOnly: true, Session: true}}}}}
}

// Real managed leases, deterministic failed/cancelled extraction: proves that
// the page adapter preserves standalone fallback semantics through logical Retry.
func TestManagedPageStandaloneRetryCancel(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in isolated managed tools")
	}
	paths := testfixture.Tools(t)
	tm := tools.New(tools.Options{Paths: paths})
	if err := tm.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := readyBrowserApp(t)
	a.downloadManager.Shutdown(context.Background())
	var mode atomic.Value
	mode.Store("edge")
	var blocked atomic.Bool
	argsSeen := make(chan []string, 16)
	run := func(ctx context.Context, exe string, args []string, _ string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) != "yt-dlp.exe" {
			return 1, errors.New("unexpected process")
		}
		argsSeen <- append([]string(nil), args...)
		if blocked.Load() {
			<-ctx.Done()
			return 1, ctx.Err()
		}
		if slices.Contains(args, "--cookies-from-browser") {
			line("stderr", "Could not copy Edge cookie database")
			return 1, errors.New("cookie access")
		}
		line("stderr", "HTTP Error 401: Unauthorized")
		return 1, errors.New("HTTP 401")
	}
	a.downloadManager = download.New(download.Options{Paths: paths, Tools: tm, Run: run, CookieMode: func() string { return mode.Load().(string) }})
	a.browserDirectory = t.TempDir()
	waitTerminal := func() download.Job {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			jobs := a.DownloadSnapshot().Jobs
			if len(jobs) > 0 && !jobs[len(jobs)-1].Active {
				return jobs[len(jobs)-1]
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatal("page job timeout")
		return download.Job{}
	}
	page := browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch?token=PAGE_SECRET"}
	if r := a.browserMessage(context.Background(), page); !r.OK {
		t.Fatal(r)
	}
	first := waitTerminal()
	if first.State != "failed" || !strings.Contains(first.Error, "cookies браузера") {
		t.Fatal(first)
	}
	checkArgs := func(wantCookies bool) {
		t.Helper()
		args := <-argsSeen
		for _, flag := range []string{"--cookies", "--add-headers", "--referer", "--user-agent"} {
			if slices.Contains(args, flag) {
				t.Fatal("captured context in page job", flag)
			}
		}
		if !slices.Contains(args, "--no-playlist") {
			t.Fatal("playlist semantics lost")
		}
		index := slices.Index(args, "--cookies-from-browser")
		if wantCookies {
			if index < 0 || args[index+1] != "edge" {
				t.Fatal("admitted cookie mode lost")
			}
		} else if index >= 0 {
			t.Fatal("first attempt used cookies")
		}
	}
	checkArgs(false)
	checkArgs(true)
	mode.Store("off")
	if _, err := a.downloadManager.Retry(first.ID); err != nil {
		t.Fatal(err)
	}
	retry := waitTerminal()
	if retry.ID != first.ID || retry.Attempt != 2 || retry.ExecutionID == first.ExecutionID {
		t.Fatal("logical retry")
	}
	checkArgs(false)
	checkArgs(true)
	blocked.Store(true)
	if r := a.browserMessage(context.Background(), page); !r.OK {
		t.Fatal(r)
	}
	<-argsSeen
	jobs := a.DownloadSnapshot().Jobs
	if _, err := a.downloadManager.Cancel(jobs[len(jobs)-1].ID); err != nil {
		t.Fatal(err)
	}
	if waitTerminal().State != "cancelled" {
		t.Fatal("page cancel")
	}
	if entries, _ := os.ReadDir(paths.Temp); len(entries) != 0 {
		t.Fatal("page temp retained")
	}
	if _, err := a.downloadManager.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPagePendingFolderRetryTrayAndShutdown(t *testing.T) {
	a := readyBrowserApp(t)
	var shown atomic.Int32
	a.showWindow = func() { shown.Add(1) }
	m := browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch?token=PAGE_SECRET"}
	if r := a.browserMessage(context.Background(), m); !r.OK {
		t.Fatal(r)
	}
	pending := a.BrowserCaptures()
	raw, _ := json.Marshal(pending)
	if len(pending) != 1 || pending[0].MediaType != "page" || strings.Contains(string(raw), "PAGE_SECRET") || shown.Load() != 1 {
		t.Fatal("page pending privacy/show")
	}
	if err := a.SetBrowserDirectory("relative/missing"); err == nil || len(a.BrowserCaptures()) != 1 {
		t.Fatal("failed folder lost page")
	}
	if err := a.SetBrowserDirectory(""); err == nil || len(a.BrowserCaptures()) != 1 {
		t.Fatal("empty folder lost page")
	}
	if err := a.SetBrowserDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.StartBrowserCapture(pending[0].ID, t.TempDir()); err == nil {
		t.Fatal("duplicate pending page")
	}
	deadline := time.Now().Add(3 * time.Second)
	for a.DownloadSnapshot().Jobs[0].Active && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	job := a.DownloadSnapshot().Jobs[0]
	if job.State != "failed" || strings.Contains(job.Source, "PAGE_SECRET") {
		t.Fatal("manager admission/redaction")
	}
	if _, err := a.downloadManager.Retry(job.ID); err != nil {
		t.Fatal(err)
	}
	if a.DownloadSnapshot().Jobs[0].Attempt != 2 {
		t.Fatal("logical retry")
	}
	if r := a.browserMessage(context.Background(), m); !r.OK || shown.Load() != 1 || len(a.DownloadSnapshot().Jobs) != 2 {
		t.Fatal("immediate tray admission", r)
	}
	a.beginShutdown()
	if len(a.BrowserCaptures()) != 0 || len(a.browserPages) != 0 {
		t.Fatal("shutdown retained page")
	}
	if r := a.browserMessage(context.Background(), m); r.OK || r.Code != "closing" {
		t.Fatal(r)
	}
}

func TestPageMixedPendingOrderAndBounds(t *testing.T) {
	a := readyBrowserApp(t)
	page := browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch"}
	for i := 0; i < 32; i++ {
		m := page
		if i%2 == 1 {
			m = appCapture()
		}
		if r := a.browserMessage(context.Background(), m); !r.OK {
			t.Fatal(r)
		}
	}
	if r := a.browserMessage(context.Background(), page); r.OK || r.Code != "busy" {
		t.Fatal("unbounded pending", r)
	}
	a.mu.Lock()
	for _, c := range a.browserCaptures {
		c.Browser.CapturedAt = time.Now().Add(-16 * time.Minute)
	}
	a.mu.Unlock()
	if len(a.BrowserCaptures()) != 16 {
		t.Fatal("pages must not use capture TTL")
	}
	if err := a.SetBrowserDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	jobs := a.DownloadSnapshot().Jobs
	if len(jobs) != 16 {
		t.Fatal("pending page count")
	}
	for i, j := range jobs {
		if j.Sequence != uint64(i+1) {
			t.Fatal("FIFO admission")
		}
	}
}
func readyBrowserApp(t *testing.T) *App {
	t.Helper()
	a := NewApp()
	a.showWindow = func() {}
	a.hideWindow = func() {}
	a.quit = func() {}
	close(a.browserReady)
	a.downloadManager = download.New(download.Options{Paths: apppaths.At(t.TempDir())})
	t.Cleanup(func() { a.downloadManager.Shutdown(context.Background()) })
	return a
}
func TestBrowserPendingFolderPrivacyAndDuplicate(t *testing.T) {
	a := readyBrowserApp(t)
	var shown atomic.Int32
	a.showWindow = func() { shown.Add(1) }
	if r := a.browserMessage(context.Background(), appCapture()); !r.OK {
		t.Fatal(r)
	}
	if shown.Load() != 1 {
		t.Fatal("folder does not show")
	}
	pending := a.BrowserCaptures()
	raw, _ := json.Marshal(pending)
	for _, secret := range []string{"COOKIE_SECRET", "HEADER_SECRET", "URL_SECRET", `"http":`, `"cookies":`} {
		if string(raw) != "" && strings.Contains(string(raw), secret) {
			t.Fatal("frontend leaked", secret)
		}
	}
	directory := t.TempDir()
	if e := a.SetBrowserDirectory(directory); e != nil {
		t.Fatal(e)
	}
	if len(a.BrowserCaptures()) != 0 || len(a.DownloadSnapshot().Jobs) != 1 {
		t.Fatal("no handoff")
	}
	if _, e := a.StartBrowserCapture(pending[0].ID, directory); e == nil {
		t.Fatal("duplicate allowed")
	}
	if r := a.browserMessage(context.Background(), appCapture()); !r.OK {
		t.Fatal(r)
	}
	if len(a.DownloadSnapshot().Jobs) != 2 || shown.Load() != 1 {
		t.Fatal("tray handoff shows window")
	}
	a.browserDirectory = ""
	a.browserMessage(context.Background(), appCapture())
	a.mu.Lock()
	for _, c := range a.browserCaptures {
		c.Browser.CapturedAt = time.Now().Add(-16 * time.Minute)
	}
	a.mu.Unlock()
	a.checkTray()
	if len(a.BrowserCaptures()) != 0 {
		t.Fatal("TTL")
	}
	_ = os.Remove(directory)
}
func TestBrowserConcurrentCaptureExit(t *testing.T) {
	a := readyBrowserApp(t)
	a.browserDirectory = t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		m := appCapture()
		if i%2 == 0 {
			m = browserbridge.Message{Version: 1, Type: "page_url", URL: "https://page.test/watch"}
		}
		wg.Add(1)
		go func() { defer wg.Done(); a.browserMessage(context.Background(), m) }()
	}
	wg.Add(1)
	go func() { defer wg.Done(); a.beginShutdown() }()
	wg.Wait()
	if r := a.browserMessage(context.Background(), appCapture()); r.OK || r.Code != "closing" {
		t.Fatal(r)
	}
	if !a.DownloadSnapshot().Closing || len(a.BrowserCaptures()) != 0 || len(a.browserPages) != 0 {
		t.Fatal("shutdown admission")
	}
}
