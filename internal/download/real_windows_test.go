//go:build windows

package download

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/tools"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func managedFixture(t *testing.T) (apppaths.Paths, *tools.Manager, map[string]string) {
	t.Helper()
	paths := testfixture.Tools(t)
	m := tools.New(tools.Options{Paths: paths})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := m.Detect(ctx); err != nil {
		t.Fatal(err)
	}
	exes := map[string]string{}
	for _, tool := range []tools.Tool{tools.YtDlp, tools.FFmpeg, tools.Deno} {
		lease, e := m.Acquire(ctx, tool)
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(lease.Close)
		for exe, path := range lease.Paths {
			exes[exe] = path
		}
	}
	return paths, m, exes
}
func generateMedia(t *testing.T, paths map[string]string, dir, audio string) string {
	t.Helper()
	path := filepath.Join(dir, "fixture.mkv")
	args := []string{"-hide_banner", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=12", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", audio, path}
	var output strings.Builder
	_, err := process.RunStreaming(context.Background(), paths["ffmpeg.exe"], args, dir, func(_, line string) {
		if output.Len() < 64000 {
			output.WriteString(line + "\n")
		}
	})
	if err != nil {
		t.Fatal(err, output.String())
	}
	return path
}
func TestManagedFFmpegFinalization(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in managed tools")
	}
	_, _, exes := managedFixture(t)
	for _, audio := range []string{"aac", "wmav2"} {
		t.Run(audio, func(t *testing.T) {
			dir := t.TempDir()
			input := generateMedia(t, exes, dir, audio)
			log, err := newLog(dir, "synthetic")
			if err != nil {
				t.Fatal(err)
			}
			defer log.close()
			var ffmpegArgs [][]string
			run := func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
				if filepath.Base(exe) == "ffmpeg.exe" {
					ffmpegArgs = append(ffmpegArgs, append([]string{}, args...))
				}
				return process.RunStreaming(ctx, exe, args, cwd, line)
			}
			output, err := finalize(context.Background(), run, exes, input, dir, log)
			if err != nil {
				t.Fatal(err)
			}
			info, err := probe(context.Background(), run, exes, output, log, true)
			if err != nil {
				t.Fatal(err)
			}
			if info.Streams[0].Codec != "h264" || info.Streams[1].Codec != "aac" {
				t.Fatal(info)
			}
			if audio == "aac" && len(ffmpegArgs) != 1 {
				t.Fatal("unnecessary encoding", ffmpegArgs)
			}
			if audio == "wmav2" {
				last := strings.Join(ffmpegArgs[len(ffmpegArgs)-1], " ")
				if !strings.Contains(last, "-c:a aac") || strings.Contains(last, "-c:v") {
					t.Fatal("selective policy", last)
				}
			}
			t.Logf("source audio=%s final=%+v ffmpeg runs=%d", audio, info.Streams, len(ffmpegArgs))
		})
	}
}
func TestManagedLocalDownload(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in managed tools")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	fixture := generateMedia(t, exes, root, "aac")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	dir := filepath.Join(root, "Результат & 100% 日本語")
	os.MkdirAll(dir, 0700)
	var captured []string
	run := func(ctx context.Context, exe string, args []string, cwd string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			captured = append([]string{}, args...)
			// Test-only localhost proxy bypass. Production builder is captured unchanged.
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, cwd, line)
	}
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run})
	initial, err := m.Start(Request{URL: server.URL + "/video.mkv?q=100%25&token=TESTSECRET", Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	select {
	case <-m.jobs[len(m.jobs)-1].done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("timeout")
	}
	s := lastJob(m)
	if s.State != "completed" {
		t.Fatal(s)
	}
	found := false
	for i, a := range captured {
		if a == "-N" && captured[i+1] == "8" {
			found = true
		}
	}
	if !found {
		t.Fatal(captured)
	}
	log, err := newLog(root, "validation")
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	if _, err = probe(context.Background(), run, exes, s.FinalPath, log, true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(m.paths.Temp, initial.Jobs[0].ExecutionID)); !os.IsNotExist(err) {
		t.Fatal("temp leak", err)
	}
	data, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", initial.Jobs[0].ExecutionID+".log"))
	if strings.Contains(string(data), "TESTSECRET") {
		t.Fatal("secret leak")
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatal(files)
	}
	t.Logf("managed engine -N 8: %+v", s)
}

func TestManagedDASHStreams(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in managed tools")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	fixture := generateMedia(t, exes, root, "aac")
	manifest := filepath.Join(root, "manifest.mpd")
	if _, err := process.RunStreaming(context.Background(), exes["ffmpeg.exe"], []string{"-nostdin", "-y", "-i", fixture, "-map", "0", "-c", "copy", "-f", "dash", manifest}, root, nil); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	dest := filepath.Join(root, "output")
	os.MkdirAll(dest, 0700)
	var phaseMu sync.Mutex
	var phases []string
	var hasPercent bool
	run := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			args = append([]string{"--proxy", ""}, args...)
		}
		return process.RunStreaming(ctx, exe, args, dir, line)
	}
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run, OnChange: func(s Snapshot) {
		phaseMu.Lock()
		defer phaseMu.Unlock()
		job := s.Jobs[0]
		phases = append(phases, job.State)
		if job.Percent != nil {
			hasPercent = true
		}
	}})
	if _, err := m.Start(Request{URL: server.URL + "/manifest.mpd", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	select {
	case <-m.jobs[len(m.jobs)-1].done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("timeout")
	}
	s := lastJob(m)
	phaseMu.Lock()
	defer phaseMu.Unlock()
	if s.State != "completed" || !hasPercent || !strings.Contains(strings.Join(phases, ","), "postprocessing") {
		t.Fatal(s, phases, hasPercent)
	}
	data, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", s.ID+".log"))
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, metaPrefix) {
			t.Log(line)
		}
	}
	t.Log("separate DASH video/audio, -N 8, merger, structured progress, final MP4 verified")
}

func TestPublicDownloadSmoke(t *testing.T) {
	source := os.Getenv("MANGAR_PUBLIC_SMOKE_URL")
	if source == "" {
		t.Skip("explicit opt-in small public video URL")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm})
	_, err := m.Start(Request{URL: source, Directory: root})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	select {
	case <-m.jobs[len(m.jobs)-1].done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("timeout")
	}
	s := lastJob(m)
	data, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", s.ID+".log"))
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, metaPrefix) || strings.Contains(line, "policy") || strings.Contains(line, "deno") {
			t.Log(line)
		}
	}
	if s.State != "completed" {
		t.Fatal(s)
	}
	log, err := newLog(root, "public-validation")
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	info, err := probe(context.Background(), process.RunStreaming, exes, s.FinalPath, log, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("public final MP4 streams=%+v", info.Streams)
}
