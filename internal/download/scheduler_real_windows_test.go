//go:build windows

package download

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagedSchedulerSixLocalJobs(t *testing.T) {
	managedSchedulerScenario(t, false)
}
func TestManagedBatchLocalJobs(t *testing.T) {
	managedSchedulerScenario(t, true)
}
func managedSchedulerScenario(t *testing.T, batch bool) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in managed tools")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	fixture := generateMedia(t, exes, root, "aac")
	gates := make([]chan struct{}, 6)
	entered := make(chan int, 100)
	for i := range gates {
		gates[i] = make(chan struct{})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		n, err := strconv.Atoi(parts[0])
		if err != nil || n < 0 || n >= 6 {
			http.NotFound(w, r)
			return
		}
		entered <- n
		select {
		case <-gates[n]:
		case <-r.Context().Done():
			return
		}
		if n == 2 {
			http.Error(w, "controlled fixture failure", http.StatusServiceUnavailable)
			return
		}
		http.ServeFile(w, r, fixture)
	}))
	defer server.Close()
	dest := filepath.Join(root, "Результат & 100% 日本語")
	os.MkdirAll(dest, 0700)
	var live, maxLive, maxLogical atomic.Int32
	starts := make(chan string, 10)
	run := func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
		n := live.Add(1)
		for old := maxLive.Load(); n > old; old = maxLive.Load() {
			if maxLive.CompareAndSwap(old, n) {
				break
			}
		}
		defer live.Add(-1)
		if filepath.Base(exe) == "yt-dlp.exe" {
			argv := strings.Join(args, "|")
			if !strings.Contains(argv, "-N|8") || !strings.Contains(argv, "-f|bv+ba/b") || !strings.Contains(argv, "--no-js-runtimes|--js-runtimes|deno:") {
				t.Error("production argv", argv)
			}
			starts <- filepath.Base(cwd)
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, cwd, line)
	}
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run, OnChange: func(s Snapshot) {
		n := int32(s.Active)
		for old := maxLogical.Load(); n > old; old = maxLogical.Load() {
			if maxLogical.CompareAndSwap(old, n) {
				break
			}
		}
	}})
	// Stop clients before the deferred server.Close, including assertion failures.
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := m.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	}()
	var jobs []Job
	if batch {
		var contents strings.Builder
		contents.WriteString("\ufeff")
		for i := range 6 {
			if i == 2 {
				contents.WriteString("invalid text\r\n\r\n")
			}
			contents.WriteString("  " + server.URL + "/" + strconv.Itoa(i) + "/fixture.mkv  \r\n")
		}
		result, err := m.ImportTXT(txtFixture(t, contents.String()), dest)
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 7 || result.Invalid != 1 || result.Queued != 6 {
			t.Fatal(result)
		}
		for _, j := range result.Snapshot.Jobs {
			if !j.InvalidSource {
				jobs = append(jobs, j)
			}
		}
	} else {
		for i := range 6 {
			s, err := m.Start(Request{URL: server.URL + "/" + strconv.Itoa(i) + "/fixture.mkv", Directory: dest})
			if err != nil {
				t.Fatal(err)
			}
			jobs = append(jobs, s.Jobs[len(s.Jobs)-1])
		}
	}
	next := func() string {
		select {
		case id := <-starts:
			return id
		case <-time.After(30 * time.Second):
			t.Fatal("real runner timeout")
			return ""
		}
	}
	for range 3 {
		next()
	}
	// Wait until all three independent yt-dlp trees are blocked on local I/O.
	seen := map[int]bool{}
	deadline := time.After(30 * time.Second)
	for len(seen) < 3 {
		select {
		case n := <-entered:
			seen[n] = true
		case <-deadline:
			t.Fatal("HTTP admission timeout")
		}
	}
	if live.Load() != 3 || m.Snapshot().Active != 3 {
		t.Fatal("three real roots", live.Load(), m.Snapshot())
	}
	close(gates[0])
	awaitState(t, m, jobs[0].ID, "completed")
	if next() != jobs[3].ExecutionID {
		t.Fatal("completion admission")
	}
	m.Cancel(jobs[1].ID)
	awaitState(t, m, jobs[1].ID, "cancelled")
	if next() != jobs[4].ExecutionID {
		t.Fatal("cancel admission")
	}
	if j := func() Job {
		for _, j := range m.Snapshot().Jobs {
			if j.ID == jobs[2].ID {
				return j
			}
		}
		return Job{}
	}(); !j.Active {
		t.Fatal("cancel affected neighbour", j)
	}
	close(gates[2])
	awaitState(t, m, jobs[2].ID, "failed")
	if next() != jobs[5].ExecutionID {
		t.Fatal("failure admission")
	}
	for _, i := range []int{3, 4, 5} {
		close(gates[i])
	}
	for _, i := range []int{3, 4, 5} {
		awaitState(t, m, jobs[i].ID, "completed")
	}
	if maxLive.Load() != 3 || maxLogical.Load() != 3 || live.Load() != 0 {
		t.Fatal("observed bounds", maxLive.Load(), maxLogical.Load(), live.Load())
	}
	entries, _ := os.ReadDir(m.paths.Temp)
	if len(entries) != 0 {
		t.Fatal("temp remnants", entries)
	}
	entries, _ = os.ReadDir(dest)
	if len(entries) != 4 {
		t.Fatal("outputs", entries)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".mp4") {
			t.Fatal("pending file", e.Name())
		}
	}
	if batch {
		for _, i := range []int{0, 3, 4, 5} {
			j := awaitState(t, m, jobs[i].ID, "completed")
			if filepath.Base(j.FinalPath) != batchName(j.BatchIndex) {
				t.Fatal(j)
			}
			log, _ := newLog(filepath.Join(root, "verification"), j.ID)
			if _, err := probe(context.Background(), run, exes, j.FinalPath, log, true); err != nil {
				t.Fatal(err)
			}
			log.close()
		}
		close(gates[1])
		if _, err := m.Retry(jobs[1].ID); err != nil {
			t.Fatal(err)
		}
		retried := awaitState(t, m, jobs[1].ID, "completed")
		if retried.BatchIndex != 2 || filepath.Base(retried.FinalPath) != "2.mp4" || retried.ExecutionID == jobs[1].ExecutionID {
			t.Fatal(retried)
		}
		t.Log("TXT real integration: seven positions, invalid #3, cancelled/retried #2, failed #4, stable N.mp4, final ffprobe valid")
	}
	t.Logf("six real local jobs: max logical=%d max simultaneous process runners=%d; middle cancelled, neighbours continued; 4 MP4, temp empty", maxLogical.Load(), maxLive.Load())
}

func TestManagedGenericHLSFallback(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in managed tools")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	input := generateMedia(t, exes, root, "aac")
	manifest := filepath.Join(root, "720.m3u8")
	args := []string{"-hide_banner", "-nostdin", "-y", "-i", input, "-c", "copy", "-hls_time", "0.5", "-hls_list_size", "0", manifest}
	if _, err := process.RunStreaming(context.Background(), exes["ffmpeg.exe"], args, root, func(string, string) {}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	dest := filepath.Join(root, "output")
	os.Mkdir(dest, 0700)
	run := func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, cwd, line)
	}
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run})
	s, err := m.Start(Request{URL: server.URL + "/720.m3u8", Directory: dest})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	select {
	case <-m.jobs[0].done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("HLS timeout")
	}
	j := m.Snapshot().Jobs[0]
	if j.State != "completed" || j.Title != "Видео — 127.0.0.1" || filepath.Base(j.FinalPath) != safeName(j.Title, s.Jobs[0].ID[:8], 150)+".mp4" {
		t.Fatal(j)
	}
	t.Logf("real HLS fallback: %s, %s", j.Title, filepath.Base(j.FinalPath))
}
