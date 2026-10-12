package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/tools"
)

func TestPublicationCancelCommitBoundary(t *testing.T) {
	for _, cancelFirst := range []bool{true, false} {
		m := New(Options{})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		j := &queuedJob{view: Job{ID: "job", Active: true, State: "finalizing"}, cancel: cancel}
		m.jobs = []*queuedJob{j}
		e := &execution{manager: m, job: j, ctx: ctx}
		renamed := false
		if cancelFirst {
			m.Cancel("job")
		}
		err := e.commit(func() error { renamed = true; return nil })
		if cancelFirst && (!errors.Is(err, context.Canceled) || renamed) {
			t.Fatal("cancel published", err, renamed)
		}
		if !cancelFirst {
			m.Cancel("job")
			if ctx.Err() != nil || j.view.State != "finalizing" {
				t.Fatal("published cancelled")
			}
		}
	}
}
func TestSchedulerLeaseAdmissionAndCleanupSlot(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
	base := m.acquire
	var acquired atomic.Int32
	releaseGate := make(chan struct{})
	releaseReached := make(chan struct{})
	var once sync.Once
	m.acquire = func(ctx context.Context, tool tools.Tool) (map[string]string, func(), error) {
		paths, release, err := base(ctx, tool)
		acquired.Add(1)
		return paths, func() {
			if tool == tools.Deno {
				once.Do(func() { close(releaseReached); <-releaseGate })
			}
			release()
		}, err
	}
	jobs := enqueue(t, m, dest, 4)
	for range 3 {
		nextRun(t, starts)
	}
	if acquired.Load() != 9 {
		t.Fatal("queued got leases", acquired.Load())
	}
	m.Cancel(jobs[1].ID)
	// A cancelled process has exited, but release is deliberately blocked.
	select {
	case <-releaseReached:
	case <-time.After(3 * time.Second):
		close(releaseGate)
		t.Fatal("lease release was not reached")
	}
	if m.Snapshot().Active != 3 || m.Snapshot().Jobs[3].State != "queued" {
		t.Fatal("slot released before leases")
	}
	close(releaseGate)
	awaitState(t, m, jobs[1].ID, "cancelled")
	nextRun(t, starts)
	if acquired.Load() != 12 {
		t.Fatal("start-time leases", acquired.Load())
	}
}
func TestRetryExternalOutputCollisionAndLogs(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
	j := enqueue(t, m, dest, 1)[0]
	nextRun(t, starts)
	m.Cancel(j.ID)
	awaitState(t, m, j.ID, "cancelled")
	name := filepath.Join(dest, safeName("Тест & Unicode 日本語", "test", 150)+".mp4")
	marker := []byte("external existing file")
	if err := os.WriteFile(name, marker, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Retry(j.ID); err != nil {
		t.Fatal(err)
	}
	r := nextRun(t, starts)
	r.release <- true
	done := awaitState(t, m, j.ID, "completed")
	data, err := os.ReadFile(name)
	if err != nil || string(data) != string(marker) || done.FinalPath == name || done.Attempt != 2 || done.ExecutionID == j.ExecutionID {
		t.Fatal("retry overwrite", done, err)
	}
	oldLog := filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log")
	if _, err := os.Stat(oldLog); err != nil {
		t.Fatal("old log lost", err)
	}
}
