package download

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConfirmedShutdownCleanupAndNoAdmission(t *testing.T) {
	for _, n := range []int{1, 3, 4, 8} {
		m, dest, starts, leases := controlledManager(t)
		jobs := enqueue(t, m, dest, n)
		for range min(n, 3) {
			nextRun(t, starts)
		}
		m.StopAdmission()
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := m.Shutdown(context.Background()); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		if m.Snapshot().Active != 0 || leases.Load() != int32(min(n, 3)*3) {
			t.Fatal(m.Snapshot(), leases.Load())
		}
		select {
		case <-starts:
			t.Fatal("queued admitted")
		default:
		}
		for _, j := range jobs {
			if _, err := os.Stat(filepath.Join(m.paths.Temp, j.ExecutionID)); !os.IsNotExist(err) {
				t.Fatal("temp remains", err)
			}
		}
		if _, err := m.Start(Request{URL: "https://host/v", Directory: dest}); err == nil {
			t.Fatal("Start after StopAdmission")
		}
		if _, err := m.Retry(jobs[0].ID); err == nil {
			t.Fatal("Retry after StopAdmission")
		}
	}
}

func TestStopIfIdleAndExitCompletionRace(t *testing.T) {
	for range 20 {
		m, dest, starts, _ := controlledManager(t)
		jobs := enqueue(t, m, dest, 1)
		r := nextRun(t, starts)
		if m.StopIfIdle() || m.Snapshot().Closing {
			t.Fatal("confirmation decision cancelled active job")
		}
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); r.release <- true }()
		go func() { defer wg.Done(); m.Cancel(jobs[0].ID) }()
		go func() { defer wg.Done(); m.Shutdown(context.Background()) }()
		wg.Wait()
		j := m.Snapshot().Jobs[0]
		if j.State != "completed" && j.State != "cancelled" {
			t.Fatal(j)
		}
		if j.State == "completed" {
			if _, err := os.Stat(j.FinalPath); err != nil {
				t.Fatal("MP4 removed", err)
			}
		}
		if _, err := os.Stat(filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log")); err != nil {
			t.Fatal("log removed", err)
		}
	}
	for _, state := range []string{"completed", "failed", "cancelled", "queued", "preparing", "fetching_info", "downloading", "postprocessing", "finalizing", "cancelling", "future"} {
		m := New(Options{})
		m.jobs = []*queuedJob{{view: Job{State: state}}}
		terminal := state == "completed" || state == "failed" || state == "cancelled"
		if m.StopIfIdle() != terminal || m.Snapshot().Closing != terminal {
			t.Fatal(state)
		}
		m.Shutdown(context.Background())
	}
}
