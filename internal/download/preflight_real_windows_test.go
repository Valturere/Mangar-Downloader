//go:build windows

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/testfixture/componentrepair"
	"YTDLP-GUI-Mangar/internal/tools"
)

type countedVerifier struct{ calls *atomic.Int32 }

func (v countedVerifier) Verify(ctx context.Context, tool tools.Tool, dir string) (map[string]string, error) {
	v.calls.Add(1)
	return (tools.ProcessVerifier{}).Verify(ctx, tool, dir)
}

func TestManagedBootPreflightControlled(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in isolated managed tools")
	}
	paths := testfixture.Tools(t)
	exes := testfixture.Executables(t, paths)
	source, h, downloads := componentrepair.DenoRepair(t, paths)
	var boot atomic.Value
	boot.Store("controlled-boot-A")
	var calls atomic.Int32
	options := tools.Options{Paths: paths, Source: source, HTTP: h, Verifier: countedVerifier{&calls}, BootIdentity: func() (string, error) { return boot.Load().(string), nil }}
	tm := tools.New(options)
	if err := tm.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	var timings []time.Duration
	for _, id := range []string{"controlled-boot-A", "controlled-boot-B", "controlled-boot-C"} {
		boot.Store(id)
		start := time.Now()
		if err := tm.Preflight(context.Background()); err != nil {
			t.Fatal(err)
		}
		timings = append(timings, time.Since(start))
	}
	if calls.Load() != 9 || downloads.Load() != 0 {
		t.Fatal("healthy preflight repeated/missed probes or used network", calls.Load(), downloads.Load())
	}
	var total time.Duration
	for _, elapsed := range timings {
		total += elapsed
	}
	t.Logf("healthy preflight: samples=%v mean=%s (local only)", timings, total/time.Duration(len(timings)))
	root := t.TempDir()
	fixture := generateMedia(t, exes, root, "aac")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	run := func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, cwd, line)
	}
	m := New(Options{Paths: paths, Tools: tm, Run: run})
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	wait := func(manager *Manager) Job {
		t.Helper()
		manager.mu.Lock()
		done := manager.jobs[len(manager.jobs)-1].done
		manager.mu.Unlock()
		select {
		case <-done:
		case <-time.After(90 * time.Second):
			t.Fatal("controlled download timeout")
		}
		j := lastJob(manager)
		if j.State != "completed" || filepath.Ext(j.FinalPath) != ".mp4" {
			t.Fatal(j)
		}
		return j
	}
	for range 2 {
		if _, err := m.Start(Request{URL: server.URL + "/video.mkv", Directory: root}); err != nil {
			t.Fatal(err)
		}
		wait(m)
	}
	if calls.Load() != 9 {
		t.Fatal("downloads repeated probes", calls.Load())
	}
	// The same on-disk marker survives a real Manager/queue shutdown and restart.
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	restart := tools.New(options)
	if err := restart.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := restart.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 9 {
		t.Fatal("restart repeated probes")
	}
	// A missing executable in this isolated profile must repair before the
	// original queued job can execute. The real binary is streamed via the manager.
	if err := os.Remove(exes["deno.exe"]); err != nil {
		t.Fatal(err)
	}
	repaired := New(Options{Paths: paths, Tools: restart, Run: run})
	t.Cleanup(func() { repaired.Shutdown(context.Background()) })
	if _, err := repaired.Start(Request{URL: server.URL + "/video.mkv", Directory: root}); err != nil {
		t.Fatal(err)
	}
	wait(repaired)
	if downloads.Load() != 1 {
		t.Fatal("repair did not install only missing Deno", downloads.Load())
	}
	// New boot, one shared check for a four-row TXT batch.
	boot.Store("controlled-boot-E")
	before := calls.Load()
	dest := t.TempDir()
	plan := txtFixture(t, server.URL+"/1.mkv\n"+server.URL+"/2.mkv\n"+server.URL+"/3.mkv\n"+server.URL+"/4.mkv")
	batch, err := repaired.ImportTXT(plan, dest)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		completed := 0
		for _, j := range repaired.Snapshot().Jobs {
			if j.BatchID == batch.BatchID && j.State == "completed" {
				completed++
			}
		}
		if completed == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("TXT timeout", repaired.Snapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load()-before != 3 {
		t.Fatal("TXT repeated probes", calls.Load()-before)
	}
	if downloads.Load() != 1 {
		t.Fatal("healthy new boot used source")
	}
	t.Log("PASS: real MP4 x2, same-boot restart, isolated missing Deno repair + pending download, changed boot, TXT shared preflight")
}
