package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type controlledRun struct {
	id, source, dest string
	release          chan bool
}

func controlledManager(t *testing.T) (*Manager, string, chan controlledRun, *atomic.Int32) {
	t.Helper()
	m, dest, leases := fixtureManager(t, "")
	base := m.run
	starts := make(chan controlledRun, 100)
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			release := make(chan bool, 1)
			starts <- controlledRun{id: filepath.Base(dir), source: args[len(args)-1], dest: dir, release: release}
			select {
			case ok := <-release:
				if !ok {
					return 7, errors.New("controlled failure")
				}
			case <-ctx.Done():
				return 1, ctx.Err()
			}
		}
		return base(ctx, exe, args, dir, line)
	}
	return m, dest, starts, leases
}
func nextRun(t *testing.T, starts chan controlledRun) controlledRun {
	t.Helper()
	select {
	case r := <-starts:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("runner did not start")
		return controlledRun{}
	}
}
func awaitState(t *testing.T, m *Manager, id, state string) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, j := range m.Snapshot().Jobs {
			if j.ID == id && j.State == state {
				return j
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not reach %s: %+v", id, state, m.Snapshot())
	return Job{}
}
func enqueue(t *testing.T, m *Manager, dest string, n int) []Job {
	t.Helper()
	var jobs []Job
	for i := range n {
		s, err := m.Start(Request{URL: "https://host/video?index=" + string(rune('a'+i)), Directory: dest})
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, s.Jobs[len(s.Jobs)-1])
	}
	return jobs
}
func TestSchedulerCapacityFIFOAndRetention(t *testing.T) {
	for _, n := range []int{1, 3, 4, 20} {
		t.Run(string(rune('a'+n)), func(t *testing.T) {
			m, dest, starts, leases := controlledManager(t)
			jobs := enqueue(t, m, dest, n)
			if s := m.Snapshot(); s.Active != min(n, 3) || len(s.Jobs) != n {
				t.Fatal(s)
			}
			running := []controlledRun{}
			for range min(n, 3) {
				running = append(running, nextRun(t, starts))
			}
			if leases.Load() != 0 {
				t.Fatal("early release")
			}
			for i := 3; i < n; i++ {
				if jobs[i].State != "queued" {
					t.Fatal(jobs[i])
				}
				if _, err := os.Stat(filepath.Join(m.paths.Temp, jobs[i].ExecutionID)); !os.IsNotExist(err) {
					t.Fatal("queued temp", err)
				}
			}
			// Finish one slot at a time. Every later admission is unambiguously FIFO.
			for i := 0; i < n; i++ {
				r := running[0]
				running = running[1:]
				r.release <- true
				var logical string
				for _, j := range jobs {
					if j.ExecutionID == r.id {
						logical = j.ID
					}
				}
				awaitState(t, m, logical, "completed")
				if i+3 < n {
					next := nextRun(t, starts)
					if next.id != jobs[i+3].ExecutionID {
						t.Fatal("FIFO", next.id, jobs[i+3])
					}
					running = append(running, next)
				}
			}
			s := m.Snapshot()
			if s.Active != 0 || len(s.Jobs) != n || leases.Load() != int32(n*3) {
				t.Fatal(s, leases.Load())
			}
			output := s.Jobs[0].FinalPath
			if _, err := m.Remove(jobs[0].ID); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(output); err != nil {
				t.Fatal("Remove deleted completed file", err)
			}
			t.Logf("jobs=%d max observed active=%d retained=%d", n, min(n, 3), len(s.Jobs))
		})
	}
}
func TestSchedulerCancelFailureRetryAndRemove(t *testing.T) {
	m, dest, starts, leases := controlledManager(t)
	jobs := enqueue(t, m, dest, 6)
	running := map[string]controlledRun{}
	for range 3 {
		r := nextRun(t, starts)
		running[r.id] = r
	}
	if _, err := m.Remove(jobs[5].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(jobs[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(jobs[1].ID); err != nil {
		t.Fatal(err)
	}
	cancelled := awaitState(t, m, jobs[1].ID, "cancelled")
	fourth := nextRun(t, starts)
	if fourth.id != jobs[3].ExecutionID {
		t.Fatal("cancel FIFO")
	}
	if s := m.Snapshot(); s.Active != 3 || leases.Load() != 3 {
		t.Fatal(s, leases.Load())
	}
	if _, err := m.Retry(cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if s := m.Snapshot(); s.Jobs[1].State != "queued" || s.Jobs[1].Attempt != 2 {
		t.Fatal(s)
	}
	running[jobs[2].ExecutionID].release <- false
	awaitState(t, m, jobs[2].ID, "failed")
	fifth := nextRun(t, starts)
	if fifth.id != jobs[4].ExecutionID {
		t.Fatal("failure FIFO")
	}
	if _, err := m.Retry(jobs[2].ID); err != nil {
		t.Fatal(err)
	}
	fourth.release <- true
	awaitState(t, m, jobs[3].ID, "completed")
	retry := nextRun(t, starts)
	if retry.id == jobs[1].ExecutionID || retry.source != running[jobs[1].ExecutionID].source {
		t.Fatal("retry identity/source", retry)
	}
	retry.release <- true
	after := awaitState(t, m, jobs[1].ID, "completed")
	if !strings.HasPrefix(after.FinalPath, dest+string(os.PathSeparator)) {
		t.Fatal("retry destination", after)
	}
	failedRetry := nextRun(t, starts)
	failedRetry.release <- true
	awaitState(t, m, jobs[2].ID, "completed")
	running[jobs[0].ExecutionID].release <- true
	fifth.release <- true
	awaitState(t, m, jobs[0].ID, "completed")
	awaitState(t, m, jobs[4].ID, "completed")
	if len(m.Snapshot().Jobs) != 5 {
		t.Fatal("removed queued started")
	}
	logs, _ := os.ReadDir(filepath.Join(m.paths.Logs, "jobs"))
	if len(logs) != 7 {
		t.Fatal("attempt logs", logs)
	}
	if _, err := m.Cancel(jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Retry(jobs[0].ID); err == nil {
		t.Fatal("completed retry")
	}
}
func TestSchedulerConcurrentEnqueueActionsSnapshots(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
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
	s := m.Snapshot()
	if s.Active != 3 || len(s.Jobs) != 20 {
		t.Fatal(s)
	}
	ids := map[string]bool{}
	for i, j := range s.Jobs {
		if ids[j.ID] || j.Sequence != uint64(i+1) {
			t.Fatal("identity/sequence", s)
		}
		ids[j.ID] = true
	}
	for range 3 {
		nextRun(t, starts)
	}
	for _, j := range s.Jobs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			m.Snapshot()
			m.Cancel(id)
			m.Retry(id)
			m.Snapshot()
			m.Remove(id)
		}(j.ID)
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if m.Snapshot().Active != 0 {
		t.Fatal(m.Snapshot())
	}
}
func TestSchedulerDestinationCapturedAndSnapshotDetached(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
	jobs := enqueue(t, m, dest, 4)
	first := nextRun(t, starts)
	nextRun(t, starts)
	nextRun(t, starts)
	other := filepath.Join(t.TempDir(), "other")
	os.Mkdir(other, 0700)
	enqueue(t, m, other, 1)
	s := m.Snapshot()
	s.Jobs[3].Title = "mutated"
	first.release <- true
	var id string
	for _, j := range jobs {
		if j.ExecutionID == first.id {
			id = j.ID
		}
	}
	awaitState(t, m, id, "completed")
	fourth := nextRun(t, starts)
	fourth.release <- true
	j := awaitState(t, m, jobs[3].ID, "completed")
	if !strings.HasPrefix(j.FinalPath, dest) || j.Title == "mutated" {
		t.Fatal(j)
	}
}
func TestCancelPhasesAndCompletionRace(t *testing.T) {
	for _, phase := range []string{"downloading", "postprocessing", "finalizing"} {
		t.Run(phase, func(t *testing.T) {
			m, dest, releases := fixtureManager(t, "")
			base := m.run
			reached := make(chan struct{})
			m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
				if (phase == "finalizing" && filepath.Base(exe) == "ffmpeg.exe") || (phase != "finalizing" && filepath.Base(exe) == "yt-dlp.exe") {
					if phase == "downloading" {
						line("stdout", progressPrefix+`{"speed":200,"eta":3}`)
					}
					if phase == "postprocessing" {
						line("stdout", postPrefix+`"processing"`)
					}
					close(reached)
					<-ctx.Done()
					return 1, ctx.Err()
				}
				return base(ctx, exe, args, dir, line)
			}
			s, err := m.Start(Request{URL: "https://host/v", Directory: dest})
			if err != nil {
				t.Fatal(err)
			}
			<-reached
			id := s.Jobs[0].ID
			m.Cancel(id)
			m.Cancel(id)
			awaitState(t, m, id, "cancelled")
			if releases.Load() != 3 {
				t.Fatal("leases")
			}
			entries, _ := os.ReadDir(dest)
			if len(entries) != 0 {
				t.Fatal("false MP4/pending", entries)
			}
			if _, err := os.Stat(filepath.Join(m.paths.Temp, id)); !os.IsNotExist(err) {
				t.Fatal("temp remains")
			}
		})
	}
	for range 20 {
		m, dest, starts, _ := controlledManager(t)
		j := enqueue(t, m, dest, 1)[0]
		r := nextRun(t, starts)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); r.release <- true }()
		go func() { defer wg.Done(); m.Cancel(j.ID) }()
		wg.Wait()
		waitJob(t, m)
		s := m.Snapshot().Jobs[0]
		if s.State != "completed" && s.State != "cancelled" {
			t.Fatal(s)
		}
		before := s.State
		m.Cancel(j.ID)
		if m.Snapshot().Jobs[0].State != before {
			t.Fatal("terminal changed")
		}
	}
}
func TestMetricTTLWithInjectedClock(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	m := New(Options{Now: func() time.Time { return now }})
	j := &queuedJob{view: Job{State: "downloading"}}
	m.jobs = []*queuedJob{j}
	e := &execution{manager: m, job: j}
	speed, eta := float64(60<<20), float64(135)
	e.progress(progress{Speed: &speed, ETA: &eta}, nil, now, true)
	now = now.Add(time.Second)
	e.progress(progress{}, nil, now, true)
	now = now.Add(time.Second)
	e.progress(progress{}, nil, now, true)
	s := m.Snapshot().Jobs[0]
	if s.Speed == nil || *s.Speed != speed || s.ETA == nil {
		t.Fatal(s)
	}
	*s.Speed = 1
	if *m.Snapshot().Jobs[0].Speed != speed {
		t.Fatal("snapshot alias")
	}
	now = now.Add(time.Second)
	s = m.Snapshot().Jobs[0]
	if s.Speed != nil || s.ETA != nil {
		t.Fatal("stale", s)
	}
	e.progress(progress{Speed: &speed, ETA: &eta}, nil, now, true)
	e.phase("postprocessing", "Объединение…")
	if s := m.Snapshot().Jobs[0]; s.Speed != nil || s.ETA != nil {
		t.Fatal("phase metrics", s)
	}
}
func TestGenericTitleFallback(t *testing.T) {
	source := "https://kodik.example/private/asset.mp4?token=SECRET"
	for _, title := range []string{"720.mp4:hls:manifest", "720 2026-10-10 18:48", "720", "master", "playlist", "index", "manifest", "video", "index.m3u8", "manifest.mpd", "asset.mp4", "asset", ""} {
		display, fallback := displayTitle(title, source)
		if !fallback || display != "Видео — kodik.example" {
			t.Fatal(title, display, fallback)
		}
	}
	for _, title := range []string{"1917", "Se7en", "Урок 1", "Оно", "Дюна", "A normal YouTube title"} {
		if genericTitle(title, source) || genericTitle(title, "https://host/"+title) {
			t.Fatal(title)
		}
	}
}
