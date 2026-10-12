package download

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/tools"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func lastJob(m *Manager) Job {
	s := m.Snapshot()
	if len(s.Jobs) == 0 {
		return Job{}
	}
	return s.Jobs[len(s.Jobs)-1]
}
func waitJob(t *testing.T, m *Manager) Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	m.mu.Lock()
	done := m.jobs[len(m.jobs)-1].done
	m.mu.Unlock()
	select {
	case <-done:
		return lastJob(m)
	case <-ctx.Done():
		t.Fatal("job timeout")
		return Job{}
	}
}
func fixtureManager(t *testing.T, mode string) (*Manager, string, *atomic.Int32) {
	t.Helper()
	root := t.TempDir()
	dest := filepath.Join(root, "Видео пробелы & 100% 日本語")
	os.MkdirAll(dest, 0700)
	releaseCount := &atomic.Int32{}
	run := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		switch filepath.Base(exe) {
		case "yt-dlp.exe":
			if mode == "blocked" {
				<-ctx.Done()
				return 1, ctx.Err()
			}
			if mode == "exit" {
				return 7, errors.New("exit 7")
			}
			line("stdout", metaPrefix+`{"id":"test","title":"Тест & Unicode 日本語"}`)
			line("stderr", progressPrefix+`{"downloaded_bytes":20,"total_bytes":100,"speed":2048,"eta":4,"format_id":"v"}`)
			if mode == "missing" {
				return 0, nil
			}
			input := filepath.Join(dir, "media.mkv")
			os.WriteFile(input, []byte("fixture"), 0600)
			raw, _ := json.Marshal(input)
			line("stdout", pathPrefix+string(raw))
			return 0, nil
		case "ffprobe.exe":
			if mode == "invalid" || (mode == "invalid-final" && filepath.Base(args[len(args)-1]) == "final.mp4") {
				line("stdout", `{"format":{"format_name":"matroska"},"streams":[]}`)
			} else {
				line("stdout", `{"format":{"format_name":"mov,mp4","tags":{"major_brand":"isom"}},"streams":[{"index":0,"codec_type":"video","codec_name":"h264"},{"index":1,"codec_type":"audio","codec_name":"aac"}]}`)
			}
		case "ffmpeg.exe":
			if mode == "blocked-finalizing" {
				<-ctx.Done()
				return 1, ctx.Err()
			}
			if mode == "missing-final" {
				return 0, nil
			}
			if mode == "ffmpeg-fail" {
				return 1, errors.New("finalization failure")
			}
			os.WriteFile(args[len(args)-1], []byte("valid-fixture"), 0600)
		}
		return 0, nil
	}
	m := New(Options{Paths: apppaths.At(root), Run: run})
	m.acquire = func(_ context.Context, tool tools.Tool) (map[string]string, func(), error) {
		if mode == "tools" {
			return nil, nil, errors.New("corrupted lease")
		}
		paths := map[string]string{}
		exes := map[tools.Tool][]string{tools.YtDlp: {"yt-dlp.exe"}, tools.FFmpeg: {"ffmpeg.exe", "ffprobe.exe"}, tools.Deno: {"deno.exe"}}
		for _, exe := range exes[tool] {
			paths[exe] = filepath.Join(root, "tools", exe)
		}
		return paths, func() { releaseCount.Add(1) }, nil
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m, dest, releaseCount
}
func TestValidation(t *testing.T) {
	for _, raw := range []string{"", "file:///C:/tool.exe", "javascript:alert(1)", `C:\tool.exe`, "https://", "https://a/%zz", "http://[invalid", "https://user:secret@host/video"} {
		if _, err := ValidateURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	raw := `https://example.com/видео?q=a&x=100%25&shell=%22%26%25%3F`
	if actual, err := ValidateURL("  " + raw + "  "); err != nil || actual != raw {
		t.Fatal(actual, err)
	}
	m, dest, _ := fixtureManager(t, "")
	if _, err := m.Start(Request{URL: raw}); err == nil {
		t.Fatal("missing folder accepted")
	}
	if _, err := m.Start(Request{URL: raw, Directory: filepath.Join(dest, "missing")}); err == nil {
		t.Fatal("invalid folder")
	}
}
func TestLifecycleFailuresAndCleanup(t *testing.T) {
	for _, mode := range []string{"", "tools", "exit", "missing", "invalid", "ffmpeg-fail", "missing-final", "invalid-final"} {
		t.Run(mode, func(t *testing.T) {
			m, dest, releases := fixtureManager(t, mode)
			var phases []string
			var mu sync.Mutex
			m.callback = func(s Snapshot) { _ = m.Snapshot(); mu.Lock(); phases = append(phases, s.Jobs[0].State); mu.Unlock() }
			initial, err := m.Start(Request{URL: "https://example.com/v?token=SUPERSECRET", Directory: dest})
			if err != nil || initial.Active == 0 {
				t.Fatal(initial, err)
			}
			final := waitJob(t, m)
			expected := "failed"
			if mode == "" {
				expected = "completed"
			}
			if final.State != expected || final.Active {
				t.Fatalf("%+v", final)
			}
			if mode != "tools" && releases.Load() != 3 {
				t.Fatal("lease not released", releases.Load())
			}
			if _, e := os.Stat(filepath.Join(m.paths.Temp, initial.Jobs[0].ExecutionID)); !os.IsNotExist(e) {
				t.Fatal("temp not cleaned", e)
			}
			files, _ := os.ReadDir(dest)
			if mode == "" && len(files) != 1 || mode != "" && len(files) != 0 {
				t.Fatal(files)
			}
			if mode == "" {
				if final.Title != "Тест & Unicode 日本語" || final.FinalPath == "" || !strings.Contains(strings.Join(phases, ","), "downloading") {
					t.Fatal(final, phases)
				}
			}
			data, e := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", initial.Jobs[0].ExecutionID+".log"))
			if e != nil || strings.Contains(string(data), "SUPERSECRET") {
				t.Fatal("log leak", e)
			}
		})
	}
}
func TestShutdownConcurrentSnapshots(t *testing.T) {
	m, dest, releases := fixtureManager(t, "blocked")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Start(Request{URL: "https://host/v", Directory: dest}); err != nil {
				t.Error(err)
			}
			m.Snapshot()
		}()
	}
	wg.Wait()
	if s := m.Snapshot(); len(s.Jobs) != 20 || s.Active != 3 {
		t.Fatal(s)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Active != 0 || releases.Load() != 9 {
		t.Fatal(m.Snapshot(), releases.Load())
	}
	for i, j := range m.Snapshot().Jobs {
		if (i < 3 && j.State != "cancelled") || (i >= 3 && j.State != "queued") {
			t.Fatal("shutdown admitted queued job", j)
		}
	}
	if _, err := m.Start(Request{URL: "https://host/v", Directory: dest}); err == nil {
		t.Fatal("start after shutdown")
	}
}
func TestProgressProtocolAndArgv(t *testing.T) {
	for _, line := range []string{"[download] 30%", progressPrefix + "garbage", progressPrefix + `{"downloaded_bytes":"bad"}`} {
		if _, _, ok := parseProgress(line); ok {
			t.Fatal(line)
		}
	}
	_, percent, ok := parseProgress(progressPrefix + `{"downloaded_bytes":50,"total_bytes_estimate":100}`)
	if !ok || percent == nil || *percent != 50 {
		t.Fatal(percent, ok)
	}
	p, percent, ok := parseProgress(progressPrefix + `{"downloaded_bytes":100,"speed":null,"eta":null}`)
	if !ok || percent != nil || p.Speed != nil || p.ETA != nil {
		t.Fatal(p, percent)
	}
	source := `https://example.org/видео?a=1&b=100%25&c=%22?`
	paths := map[string]string{"ffmpeg.exe": `C:\managed & 100%\ffmpeg.exe`, "deno.exe": `C:\managed\deno.exe`}
	args := ytdlpArgs(paths, `C:\temp`, source)
	found := false
	for i, v := range args {
		if v == "-N" && args[i+1] == "8" {
			found = true
		}
	}
	if !found || args[len(args)-1] != source || args[len(args)-2] != "--" {
		t.Fatal(args)
	}
	if !strings.Contains(strings.Join(args, "|"), "deno:"+paths["deno.exe"]) {
		t.Fatal(args)
	}
}
func TestRedactionAndCollision(t *testing.T) {
	for _, raw := range []string{`{"Authorization":"Bearer SECRET"}`, `{'Cookie': 'session=SECRET'}`, `"x-api-key": "SECRET"`, `token=SECRET`} {
		if strings.Contains(redact(raw), "SECRET") {
			t.Fatal("quoted credential leak", redact(raw))
		}
	}
	raw := `https://user:SECRET@host/path/PRIVATE?token=SIGNED#FRAGMENT Authorization: Bearer BEARER`
	result := redact(raw)
	for _, secret := range []string{"SECRET", "PRIVATE", "SIGNED", "FRAGMENT", "BEARER"} {
		if strings.Contains(result, secret) {
			t.Fatal(result)
		}
	}
	m, dest, _ := fixtureManager(t, "")
	r := Request{URL: "https://host/v", Directory: dest}
	if _, err := m.Start(r); err != nil {
		t.Fatal(err)
	}
	first := waitJob(t, m)
	if _, err := m.Start(r); err != nil {
		t.Fatal(err)
	}
	second := waitJob(t, m)
	if first.FinalPath == second.FinalPath || !strings.HasSuffix(second.FinalPath, " (2).mp4") {
		t.Fatal(first, second)
	}
	if name := safeName(`CON<>:"/\|?* 日本語`, "id", 100); strings.ContainsAny(name, `<>:"/\|?*`) {
		t.Fatal(name)
	}
}

func TestFinalizationIOFailureNeverEncodes(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	baseRun := m.run
	var ffmpegCalls int
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "ffmpeg.exe" {
			ffmpegCalls++
			if strings.Contains(strings.Join(args, " "), "libx264") || strings.Contains(strings.Join(args, " "), "-c:a aac") {
				t.Error("I/O failure triggered encoding")
			}
			return 1, errors.New("disk full")
		}
		return baseRun(ctx, exe, args, dir, line)
	}
	if _, err := m.Start(Request{URL: "https://host/v", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	if s := waitJob(t, m); s.State != "failed" || ffmpegCalls != 2 {
		t.Fatal(s, ffmpegCalls)
	}
}

func TestShutdownDuringFinalization(t *testing.T) {
	m, dest, releases := fixtureManager(t, "blocked-finalizing")
	reached := make(chan struct{})
	var once sync.Once
	m.callback = func(s Snapshot) {
		if s.Jobs[0].State == "finalizing" {
			once.Do(func() { close(reached) })
		}
	}
	initial, err := m.Start(Request{URL: "https://host/v", Directory: dest})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(3 * time.Second):
		t.Fatal("finalization not reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Active != 0 || releases.Load() != 3 {
		t.Fatal("shutdown/leases")
	}
	if _, err = os.Stat(filepath.Join(m.paths.Temp, initial.Jobs[0].ExecutionID)); !os.IsNotExist(err) {
		t.Fatal("temp remains", err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatal(entries)
	}
}

func TestPublicationFailureLeavesNoPendingFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.mp4")
	if err := os.WriteFile(source, []byte("validated media"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publish(ctx, source, dest, "title", "id", "job"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 0 {
		t.Fatal("pending left", entries)
	}
	if _, err := publish(context.Background(), source, filepath.Join(root, "missing"), "title", "id", "job"); err == nil {
		t.Fatal("missing destination accepted")
	}
}

func TestManagedLeaseValidationAndPartialRelease(t *testing.T) {
	for _, mode := range []string{"relative", "mixed-pair", "unavailable-second"} {
		t.Run(mode, func(t *testing.T) {
			m, dest, releases := fixtureManager(t, "")
			baseAcquire := m.acquire
			m.acquire = func(ctx context.Context, tool tools.Tool) (map[string]string, func(), error) {
				if tool == tools.FFmpeg && mode == "unavailable-second" {
					return nil, nil, errors.New("missing FFmpeg")
				}
				paths, release, err := baseAcquire(ctx, tool)
				if tool == tools.FFmpeg {
					if mode == "relative" {
						paths["ffmpeg.exe"] = "ffmpeg.exe"
					}
					if mode == "mixed-pair" {
						paths["ffprobe.exe"] = filepath.Join(m.paths.Root, "other-version", "ffprobe.exe")
					}
				}
				return paths, release, err
			}
			if _, err := m.Start(Request{URL: "https://host/v", Directory: dest}); err != nil {
				t.Fatal(err)
			}
			if s := waitJob(t, m); s.State != "failed" || s.Active {
				t.Fatal(s)
			}
			want := int32(3)
			if mode == "unavailable-second" {
				want = 1
			}
			if releases.Load() != want {
				t.Fatal("partial lease leak", releases.Load())
			}
			entries, _ := os.ReadDir(dest)
			if len(entries) != 0 {
				t.Fatal("output from invalid lease", entries)
			}
		})
	}
}
