package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/download"
	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/testfixture/componentrepair"
	"YTDLP-GUI-Mangar/internal/tools"
)

func TestManagedBrowserSharedPreparation(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in isolated managed tools")
	}
	paths := testfixture.Tools(t)
	exes := testfixture.Executables(t, paths)
	source, h, installations := componentrepair.DenoRepair(t, paths)
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var sourceOnce sync.Once
	var checks atomic.Int32
	tm := tools.New(tools.Options{Paths: paths, HTTP: h, Context: ctx, BootIdentity: func() (string, error) { return "browser-controlled-boot", nil }, Source: componentrepair.SourceFunc(func(ctx context.Context, tool tools.Tool) (tools.Candidate, error) {
		sourceOnce.Do(func() { close(entered) })
		select {
		case <-release:
		case <-ctx.Done():
			return tools.Candidate{}, ctx.Err()
		}
		return source.Latest(ctx, tool)
	})})
	root := t.TempDir()
	fixture := filepath.Join(root, "video.mp4")
	if _, err := process.RunStreaming(ctx, exes["ffmpeg.exe"], []string{"-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=12", "-t", "1", "-c:v", "libx264", fixture}, root, nil); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	if err := os.Remove(exes["deno.exe"]); err != nil {
		t.Fatal(err)
	}
	a := readyBrowserApp(t)
	a.downloadManager.Shutdown(ctx)
	a.browserDirectory = t.TempDir()
	a.downloadManager = download.New(download.Options{Paths: paths, Tools: tm, Run: func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			checks.Add(1)
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, cwd, line)
	}})
	page := browserbridge.Message{Version: 1, Type: "page_url", URL: server.URL + "/page.mp4"}
	capture := browserbridge.Message{Version: 1, Type: "media_capture", Capture: &browserbridge.Capture{URL: server.URL + "/capture.mp4", TabID: 1, HTTP: download.HTTPContext{Captured: true}, Browser: download.BrowserContext{PageTitle: "Контролируемый MP4", MediaType: "mp4", ExactCookies: true, CapturedAt: time.Now()}}}
	id, err := browserbridge.UserIdentity(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pipe, err := browserbridge.Listen(id, a.browserMessage)
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	for _, request := range []browserbridge.Message{page, capture, page} {
		start := time.Now()
		if reply, err := browserbridge.Send(ctx, id, request); err != nil || !reply.OK {
			t.Fatal(reply, err)
		}
		if time.Since(start) > time.Second {
			t.Fatal("handoff blocked on component download")
		}
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("repair not started")
	}
	if checks.Load() != 0 || len(a.DownloadSnapshot().Jobs) != 3 {
		t.Fatal("browser intent lost/media ran before repair", checks.Load(), a.DownloadSnapshot())
	}
	close(release)
	deadline := time.Now().Add(90 * time.Second)
	for {
		completed := 0
		for _, job := range a.DownloadSnapshot().Jobs {
			if job.State == "failed" {
				t.Fatal(job)
			}
			if job.State == "completed" {
				completed++
			}
		}
		if completed == 3 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("browser repair timeout", a.DownloadSnapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if installations.Load() != 1 || checks.Load() != 3 || len(a.BrowserCaptures()) != 0 {
		t.Fatal("duplicate repair/request", installations.Load(), checks.Load())
	}
	t.Log("PASS: 3 framed Windows named-pipe browser handoffs (page_url/media_capture), immediate protocol replies, one repair, original MP4 downloads completed")
}
