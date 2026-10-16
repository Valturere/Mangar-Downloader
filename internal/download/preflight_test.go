package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestPreflightFailureRetainsRequestAndRetry(t *testing.T) {
	m, dest, leases := fixtureManager(t, "")
	var healthy atomic.Bool
	var checks atomic.Int32
	m.preflight = func(context.Context) error {
		checks.Add(1)
		if !healthy.Load() {
			return errors.New("repair failed")
		}
		return nil
	}
	s, err := m.Start(Request{URL: "https://example.test/watch?token=SECRET", Directory: dest})
	if err != nil {
		t.Fatal(err)
	}
	j := waitJob(t, m)
	if j.State != "failed" || leases.Load() != 0 {
		t.Fatal("engine ran before preflight", j, leases.Load())
	}
	healthy.Store(true)
	if _, err := m.Retry(s.Jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	j = waitJob(t, m)
	if j.State != "completed" || j.Attempt != 2 || checks.Load() != 2 {
		t.Fatal(j, checks.Load())
	}
}

func TestPreflightHoldsTXTIntentWithoutRunningMedia(t *testing.T) {
	m, dest, leases := fixtureManager(t, "")
	entered, release := make(chan struct{}, 3), make(chan struct{})
	m.preflight = func(ctx context.Context) error {
		entered <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	path := txtFixture(t, "https://example.test/one\nhttps://example.test/two\nhttps://example.test/three\nhttps://example.test/four")
	batch, err := m.ImportTXT(path, dest)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		<-entered
	}
	if leases.Load() != 0 || len(m.Snapshot().Jobs) != 4 || m.Snapshot().Active != 3 {
		t.Fatal("pending intent/slot policy lost")
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 0 {
		t.Fatal("media started before preparation")
	}
	close(release)
	for _, job := range batch.Snapshot.Jobs {
		m.mu.Lock()
		done := m.findLocked(job.ID).done
		m.mu.Unlock()
		if done != nil {
			<-done
		}
	}
	// The fourth job is scheduled after the first worker finishes.
	last := waitJob(t, m)
	if last.State != "completed" {
		t.Fatal(last)
	}
	for i := 1; i <= 4; i++ {
		if _, err := os.Stat(filepath.Join(dest, batchName(i))); err != nil {
			t.Fatal(err)
		}
	}
}
