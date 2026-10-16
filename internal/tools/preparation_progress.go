package tools

import (
	"math"
	"time"
)

// PreparationProgress contains only presentation-safe measurements for one
// current artifact. Steps describe phases, never fractions of total bytes.
type PreparationProgress struct {
	Phase           string    `json:"phase"`
	DownloadedBytes int64     `json:"downloadedBytes"`
	TotalBytes      *int64    `json:"totalBytes"`
	BytesPerSecond  *float64  `json:"bytesPerSecond"`
	ETASeconds      *float64  `json:"etaSeconds"`
	Step            int       `json:"step"`
	TotalSteps      int       `json:"totalSteps"`
	Indeterminate   bool      `json:"indeterminate"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type preparationTracker struct {
	view             PreparationProgress
	tool             Tool
	started, sampled time.Time
	bytes            int64
	samples          int
	speed, deviation float64
}

func (p *preparationTracker) phase(phase string) {
	*p = preparationTracker{}
	step := map[string]int{"checking": 1, "preparing": 2, "finishing": 3}[phase]
	p.view = PreparationProgress{Phase: phase, Step: step, TotalSteps: 3, Indeterminate: true}
}

func (p *preparationTracker) update(s Status, now time.Time) {
	if s.State != "downloading" {
		p.phase("preparing")
		return
	}
	n := max(int64(0), s.Bytes)
	if p.tool != s.Tool || p.started.IsZero() || n < p.bytes {
		p.phase("preparing")
		p.tool, p.started, p.sampled, p.bytes = s.Tool, now, now, n
	}
	v := &p.view
	v.DownloadedBytes, v.UpdatedAt = n, now.UTC()
	v.TotalBytes = nil
	v.Indeterminate = s.Total <= 0
	if s.Total > 0 {
		total := s.Total
		v.TotalBytes = &total
	}
	if elapsed := now.Sub(p.sampled).Seconds(); elapsed >= 0.5 {
		rate := float64(n-p.bytes) / elapsed
		if p.samples == 0 {
			p.speed = rate
		} else {
			p.speed = 0.25*rate + 0.75*p.speed
			p.deviation = 0.25*math.Abs(rate-p.speed) + 0.75*p.deviation
		}
		p.samples++
		p.sampled, p.bytes = now, n
		v.BytesPerSecond, v.ETASeconds = nil, nil
		if rate > 0 && p.speed > 0 {
			speed := p.speed
			v.BytesPerSecond = &speed
			if s.Total > n && p.samples >= 4 && now.Sub(p.started) >= 2*time.Second && p.deviation/p.speed <= 0.3 {
				eta := float64(s.Total-n) / p.speed
				v.ETASeconds = &eta
			}
		}
	}
	if s.Total > 0 && n >= s.Total {
		v.BytesPerSecond, v.ETASeconds = nil, nil
	}
}
