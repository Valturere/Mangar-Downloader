//go:build windows

package download

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
)

// Explicit runtime-only input; never print request/metadata/argv or retain media.
func TestRuntimeDirectMediaSmoke(t *testing.T) {
	raw := os.Getenv("MANGAR_DIRECT_MEDIA_SMOKE_URL")
	if raw == "" {
		t.Skip("runtime-only fresh signed URL")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm})
	defer m.Shutdown(context.Background())
	var httpContext *HTTPContext
	if origin := os.Getenv("MANGAR_DIRECT_MEDIA_SMOKE_ORIGIN"); origin != "" {
		httpContext = &HTTPContext{Origin: origin}
		httpContext.UserAgent = os.Getenv("MANGAR_DIRECT_MEDIA_SMOKE_UA")
		httpContext.Referer = os.Getenv("MANGAR_DIRECT_MEDIA_SMOKE_REFERER")
	}
	if _, err := m.Start(Request{URL: raw, Directory: root, HTTPContext: httpContext}); err != nil {
		t.Fatal("request rejected")
	}
	m.mu.Lock()
	done := m.jobs[0].done
	m.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("direct media timeout")
	}
	j := lastJob(m)
	if j.State != "completed" {
		b, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log"))
		var messages []string
		for _, line := range strings.Split(string(b), "\n") {
			var entry struct {
				Stream string `json:"stream"`
				Detail string `json:"detail"`
			}
			if json.Unmarshal([]byte(line), &entry) == nil && entry.Stream == "stderr" && strings.Contains(entry.Detail, "ERROR") {
				messages = append(messages, redact(entry.Detail))
			}
		}
		for _, message := range messages[max(0, len(messages)-4):] {
			t.Log(message)
		}
		t.Fatal("direct media failed:", j.Error)
	}
	log, err := newLog(root, "validation")
	if err != nil {
		t.Fatal("validation log")
	}
	defer log.close()
	info, err := probe(ctx, process.RunStreaming, exes, j.FinalPath, log, true)
	if err != nil {
		t.Fatal("final MP4 invalid")
	}
	b, err := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log"))
	if err != nil {
		t.Fatal("missing technical log")
	}
	if strings.Contains(string(b), raw) || (httpContext != nil && strings.Contains(string(b), httpContext.Origin)) {
		t.Fatal("private context leaked")
	}
	if _, err = os.Stat(filepath.Join(m.paths.Temp, j.ExecutionID)); !os.IsNotExist(err) {
		t.Fatal("temp leak")
	}
	u, _ := url.Parse(raw)
	file, _ := os.Stat(j.FinalPath)
	t.Logf("host=%s completed; MP4 bytes=%d streams=%+v; temp clean; URL/context redacted", u.Hostname(), file.Size(), info.Streams)
}
