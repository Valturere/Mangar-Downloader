package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func readyFixture(t *testing.T) (*fixture, *atomic.Int32) {
	t.Helper()
	f := newFixture(t)
	if err := f.m.InstallNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.m.bootIdentity = func() (string, error) { return "boot-A", nil }
	calls := &atomic.Int32{}
	v := f.m.verifier
	f.m.verifier = verifierFunc(func(ctx context.Context, tool Tool, dir string) (map[string]string, error) {
		calls.Add(1)
		return v.Verify(ctx, tool, dir)
	})
	return f, calls
}

func TestBootPreflightLocalRestartAndChangedBoot(t *testing.T) {
	f, calls := readyFixture(t)
	ctx := context.Background()
	// Both version lookup and artifact HTTP would fail this healthy test.
	f.m.source = sourceFunc(func(context.Context, Tool) (Candidate, error) {
		t.Error("healthy preflight used network")
		return Candidate{}, errors.New("offline")
	})
	before := len(f.requests)
	if err := f.m.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("startup probed healthy tools")
	}
	if err := f.m.Preflight(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("four executable probes (FFmpeg pair) expected", calls.Load())
	}
	for range 3 {
		if err := f.m.Preflight(ctx); err != nil {
			t.Fatal(err)
		}
		for _, tool := range toolOrder {
			lease, err := f.m.Acquire(ctx, tool)
			if err != nil {
				t.Fatal(err)
			}
			lease.Close()
		}
	}
	if calls.Load() != 3 {
		t.Fatal("second download repeated probes", calls.Load())
	}
	restart := New(Options{Paths: f.m.paths, Verifier: f.m.verifier, Source: f.m.source, BootIdentity: f.m.bootIdentity})
	if err := restart.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := restart.Preflight(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("same boot restart repeated probes")
	}
	restart.bootIdentity = func() (string, error) { return "boot-B", nil }
	if err := restart.Preflight(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 6 {
		t.Fatal("new boot was not checked", calls.Load())
	}
	if len(f.requests) != before {
		t.Fatal("healthy network request")
	}
}

func TestPreflightRepairsMissingAndBrokenCurrent(t *testing.T) {
	for _, mode := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe", "deno.exe", "current", "immutable", "version-dir", "probe-fail", "probe-timeout"} {
		t.Run(mode, func(t *testing.T) {
			f, _ := readyFixture(t)
			tool := FFmpeg
			if mode == "yt-dlp.exe" {
				tool = YtDlp
			}
			if mode == "deno.exe" || mode == "probe-fail" || mode == "probe-timeout" {
				tool = Deno
			}
			man := f.m.active[tool]
			dir := f.m.versionDir(tool, man.Identity)
			var err error
			switch mode {
			case "current":
				err = os.WriteFile(filepath.Join(f.m.toolDir(tool), "current.json"), []byte("broken"), 0600)
			case "immutable":
				err = os.WriteFile(filepath.Join(dir, "manifest.json"), []byte("broken"), 0600)
			case "version-dir":
				err = os.RemoveAll(dir)
			case "probe-fail", "probe-timeout":
				f.verifyFailure = func(t Tool, p string) error {
					if t == tool && p == dir {
						if mode == "probe-timeout" {
							return context.DeadlineExceeded
						}
						return errors.New("injected version probe failure")
					}
					return nil
				}
			default:
				err = os.Remove(filepath.Join(dir, mode))
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := f.m.Preflight(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertInstalled(t, f.m)
			for _, other := range toolOrder {
				want := 1
				if other == tool {
					want = 2
				}
				if f.requests[other] != want {
					t.Fatal("unnecessary download or missing repair", f.requests)
				}
			}
			if !f.m.markerMatches("boot-A", mustFingerprint(t, f.m)) {
				t.Fatal("repair did not certify")
			}
		})
	}
}
func mustFingerprint(t *testing.T, m *Manager) string {
	t.Helper()
	fingerprint, err := m.localState(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func TestFirstRunPartialFailureAndRetry(t *testing.T) {
	f := newFixture(t)
	f.m.bootIdentity = func() (string, error) { return "boot-A", nil }
	f.verifyFailure = func(tool Tool, _ string) error {
		if tool == FFmpeg {
			return errors.New("offline fixture")
		}
		return nil
	}
	if err := f.m.Prepare(context.Background()); err == nil {
		t.Fatal("broken install passed")
	}
	if f.m.Snapshot().Preparation != "failed" {
		t.Fatal("missing failure UI state")
	}
	if _, err := os.Stat(f.m.markerPath()); !os.IsNotExist(err) {
		t.Fatal("failed preparation saved marker")
	}
	f.verifyFailure = nil
	if err := f.m.RetryPreparation(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertInstalled(t, f.m)
	if f.requests[YtDlp] != 1 || f.requests[Deno] != 1 || f.m.Snapshot().Preparation != "" {
		t.Fatal("Retry did not reuse partial install", f.requests)
	}
}

func TestPreflightSingleflightAndCancelledWaiter(t *testing.T) {
	f, calls := readyFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	v := f.m.verifier
	f.m.verifier = verifierFunc(func(ctx context.Context, tool Tool, dir string) (map[string]string, error) {
		once.Do(func() { close(entered); <-release })
		return v.Verify(ctx, tool, dir)
	})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- f.m.Preflight(ctx) }()
	<-entered
	const count = 16
	errs := make(chan error, count)
	for range count {
		go func() { errs <- f.m.Preflight(context.Background()) }()
	}
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(release)
	for range count {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("concurrent probes", calls.Load())
	}
}

func TestPreflightRepairFailureSharedAndNoMarker(t *testing.T) {
	f, _ := readyFixture(t)
	originalSource := f.m.source
	man := f.m.active[YtDlp]
	if err := os.Remove(filepath.Join(f.m.versionDir(YtDlp, man.Identity), "yt-dlp.exe")); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	f.m.source = sourceFunc(func(context.Context, Tool) (Candidate, error) {
		close(entered)
		<-release
		return Candidate{}, errors.New("offline")
	})
	errs := make(chan error, 3)
	go func() { errs <- f.m.Preflight(context.Background()) }()
	<-entered
	for range 2 {
		go func() { errs <- f.m.Preflight(context.Background()) }()
	}
	// Ensure waiters have joined the blocked operation before releasing it.
	time.Sleep(50 * time.Millisecond)
	close(release)
	for range 3 {
		if err := <-errs; err == nil {
			t.Fatal("failed repair passed")
		}
	}
	for range 5 {
		if err := f.m.Preflight(context.Background()); err == nil {
			t.Fatal("queued request did not share failed wave")
		}
	}
	if _, err := os.Stat(f.m.markerPath()); !os.IsNotExist(err) {
		t.Fatal("failed repair saved marker")
	}
	f.m.source = originalSource
	if err := f.m.RetryPreparation(context.Background()); err != nil {
		t.Fatal("explicit Retry did not reset failure", err)
	}
}

func TestCleanPreparationAndManualMaintenance(t *testing.T) {
	f := newFixture(t)
	f.m.bootIdentity = func() (string, error) { return "boot-A", nil }
	if err := f.m.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertInstalled(t, f.m)
	for _, tool := range toolOrder {
		if f.requests[tool] != 1 {
			t.Fatal(f.requests)
		}
	}
	if err := f.m.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.version = 2
	if err := f.m.CheckUpdates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := f.m.InstallNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Preflight(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertInstalled(t, f.m)
	for _, tool := range toolOrder {
		if f.requests[tool] != 2 {
			t.Fatal("manual update broken", f.requests)
		}
	}
}

func TestAutomaticShutdownAwaitsRepair(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.m.context = ctx
	entered := make(chan struct{})
	f.m.source = sourceFunc(func(ctx context.Context, tool Tool) (Candidate, error) {
		close(entered)
		<-ctx.Done()
		return Candidate{}, ctx.Err()
	})
	done := make(chan error, 1)
	go func() { done <- f.m.Prepare(ctx) }()
	<-entered
	cancel()
	<-done
	wait, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err := f.m.WaitAutomatic(wait); err != nil {
		t.Fatal(err)
	}
	if f.m.Snapshot().Busy {
		t.Fatal("repair outlived shutdown")
	}
}

func TestBootIdentityFailureAndStaleMarkerFailClosed(t *testing.T) {
	f, calls := readyFixture(t)
	f.m.bootIdentity = func() (string, error) { return "", errors.New("native failure") }
	if err := f.m.Preflight(context.Background()); err == nil {
		t.Fatal("missing identity passed")
	}
	if calls.Load() != 0 {
		t.Fatal("identity failure ran tools")
	}
	f.m.bootIdentity = func() (string, error) { return "boot-A", nil }
	if err := f.m.saveMarker(verificationMarker{Schema: 1, Boot: "boot-A", Components: "stale"}); err != nil {
		t.Fatal(err)
	}
	if err := f.m.RetryPreparation(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("stale marker skipped probes")
	}
}

func TestWindowsBootIdentityStable(t *testing.T) {
	a, err := windowsBootIdentity()
	if err != nil {
		t.Fatal(err)
	}
	b, err := windowsBootIdentity()
	if err != nil || a == "" || a != b {
		t.Fatal(a, b, err)
	}
}

func TestPrepareRetainsStartupGCProtection(t *testing.T) {
	for _, protected := range []string{"orphan", "previous", "lease"} {
		t.Run(protected, func(t *testing.T) {
			f, candidate := gcFixture(t)
			if protected == "previous" {
				gcPointer(t, f.m, "previous.json", candidate)
			}
			if protected == "lease" {
				f.m.active[YtDlp] = candidate
				lease, err := f.m.Acquire(context.Background(), YtDlp)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
			}
			if err := f.m.Prepare(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertGCVersion(t, f.m, candidate, protected != "orphan")
			assertGCVersion(t, f.m, f.m.active[YtDlp], true)
		})
	}
}
