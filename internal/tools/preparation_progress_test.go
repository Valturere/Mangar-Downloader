package tools

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"
)

func TestPreparationProgressStableSpeedETAAndReset(t *testing.T) {
	var p preparationTracker
	p.phase("preparing")
	start := time.Unix(100, 0)
	const mb = int64(1024 * 1024)
	for i := 0; i <= 4; i++ {
		p.update(Status{Tool: FFmpeg, State: "downloading", Bytes: int64(i) * 10 * mb, Total: 100 * mb}, start.Add(time.Duration(i)*time.Second))
		if i < 4 && p.view.ETASeconds != nil {
			t.Fatal("premature ETA", i)
		}
	}
	if p.view.Indeterminate || *p.view.TotalBytes != 100*mb || p.view.Step != 2 || p.view.TotalSteps != 3 {
		t.Fatal(p.view)
	}
	if p.view.BytesPerSecond == nil || *p.view.BytesPerSecond != float64(10*mb) || p.view.ETASeconds == nil || *p.view.ETASeconds != 6 {
		t.Fatal("stable sample not measured", p.view)
	}
	previous := *p.view.ETASeconds
	p.update(Status{Tool: FFmpeg, State: "downloading", Bytes: 51 * mb, Total: 100 * mb}, start.Add(5*time.Second))
	if p.view.ETASeconds == nil || math.Abs(*p.view.ETASeconds-previous) > 2 {
		t.Fatal("ETA jitter")
	}
	if *p.view.BytesPerSecond >= float64(11*mb) {
		t.Fatal("speed not smoothed")
	}
	// A sudden burst is not a reliable ETA estimate.
	p.update(Status{Tool: FFmpeg, State: "downloading", Bytes: 90 * mb, Total: 100 * mb}, start.Add(6*time.Second))
	if p.view.ETASeconds != nil {
		t.Fatal("unstable ETA shown")
	}
	p.update(Status{Tool: FFmpeg, State: "verifying"}, start.Add(7*time.Second))
	if !p.view.Indeterminate || p.view.BytesPerSecond != nil || p.view.ETASeconds != nil || p.view.DownloadedBytes != 0 {
		t.Fatal("download metrics leaked into verification")
	}
	p.update(Status{Tool: Deno, State: "downloading", Bytes: 0, Total: 75 * mb}, start.Add(8*time.Second))
	if p.view.BytesPerSecond != nil || p.view.ETASeconds != nil || p.view.DownloadedBytes != 0 {
		t.Fatal("new artifact reused speed")
	}
	p.phase("failed")
	if p.view.BytesPerSecond != nil || p.view.ETASeconds != nil {
		t.Fatal("failed progress still running")
	}
	p.phase("checking")
	if p.view.Step != 1 || p.view.DownloadedBytes != 0 {
		t.Fatal("Retry did not reset")
	}
	p.phase("finishing")
	if p.view.Step != 3 || !p.view.Indeterminate {
		t.Fatal("finishing is not indeterminate")
	}
}

func TestPreparationProgressUnknownTotalAndPrivacy(t *testing.T) {
	for _, total := range []int64{0, -1} {
		var p preparationTracker
		p.phase("preparing")
		start := time.Unix(100, 0)
		for i := 0; i <= 4; i++ {
			p.update(Status{Tool: YtDlp, State: "downloading", Bytes: int64(i) * 1024 * 1024, Total: total, Message: "https://private.test/?token=SECRET cookie=SECRET"}, start.Add(time.Duration(i)*time.Second))
		}
		if p.view.TotalBytes != nil || !p.view.Indeterminate || p.view.ETASeconds != nil || p.view.BytesPerSecond == nil {
			t.Fatal(p.view)
		}
		raw, err := json.Marshal(p.view)
		if err != nil || strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "private.test") || strings.Contains(string(raw), "yt-dlp") {
			t.Fatal("unsafe progress DTO", string(raw), err)
		}
	}
}

func TestSharedPreparationProgressLifecycle(t *testing.T) {
	f := newFixture(t)
	var phases []string
	f.m.callback = func(s Snapshot) {
		if s.Progress != nil {
			phases = append(phases, s.Progress.Phase)
		}
	}
	if err := f.m.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := f.m.Snapshot(); s.Progress != nil || s.Preparation != "" {
		t.Fatal("success did not close progress", s)
	}
	joined := strings.Join(phases, ",")
	for _, phase := range []string{"checking", "preparing", "finishing"} {
		if !strings.Contains(joined, phase) {
			t.Fatal("missing real phase", phase, joined)
		}
	}
}
