package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestStandaloneBoundedFallback(t *testing.T) {
	for _, tc := range []struct {
		name, mode, first, second string
		calls                     int
	}{
		{"normal", "auto", "", "", 1}, {"404", "auto", "HTTP Error 404", "", 1}, {"DRM", "auto", "DRM protected", "", 1},
		{"off", "off", "HTTP Error 403", "", 1}, {"auto", "auto", "HTTP Error 401", "", 2},
		{"chrome", "chrome", "Login required", "", 2}, {"edge", "edge", "use --cookies", "", 2}, {"firefox", "firefox", "sign in", "", 2},
		{"cookie-denied", "chrome", "HTTP Error 403", "Could not copy Chrome cookie database C:\\PRIVATE_PROFILE\\Cookies", 2},
		{"bounded403", "edge", "HTTP Error 403", "HTTP Error 403", 3},
		{"numeric-path", "auto", "No such file or directory: media403.part", "", 1},
		{"http403-signed404", "auto", "HTTP Error 403: https://cdn.test/404.m3u8", "", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, d, _ := fixtureManager(t, "normal")
			base := m.run
			m.cookieMode = func() string { return tc.mode }
			m.detectBrowser = func() string { return "firefox" }
			calls := 0
			m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
				if filepath.Base(exe) != "yt-dlp.exe" {
					return base(ctx, exe, args, dir, line)
				}
				if slices.Contains(args, "--list-impersonate-targets") {
					line("stdout", "Chrome-116 Windows-10 curl_cffi")
					return 0, nil
				}
				calls++
				if calls == 1 {
					if slices.Contains(args, "--cookies-from-browser") {
						t.Fatal("first uses cookies")
					}
					if tc.first != "" {
						line("stderr", tc.first)
						return 1, errors.New(tc.first)
					}
				}
				if calls >= 2 {
					i := slices.Index(args, "--cookies-from-browser")
					want := tc.mode
					if want == "auto" {
						want = "firefox"
					}
					if i < 0 || args[i+1] != want {
						t.Fatal("selected browser")
					}
					if tc.second != "" {
						line("stderr", tc.second)
						return 1, errors.New(tc.second)
					}
				}
				return base(ctx, exe, args, dir, line)
			}
			s, err := m.Start(Request{URL: "https://media.test/video", Directory: d})
			if err != nil {
				t.Fatal(err)
			}
			job := waitJob(t, m)
			if calls != tc.calls {
				t.Fatalf("attempts=%d want %d", calls, tc.calls)
			}
			if tc.name == "cookie-denied" && !strings.Contains(job.Error, "cookies браузера") {
				t.Fatal(job.Error)
			}
			logs, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", job.ExecutionID+".log"))
			if strings.Contains(string(logs), "PRIVATE_PROFILE") {
				t.Fatal("profile leaked")
			}
			// Logical Retry starts from ordinary extraction, retaining the admitted mode.
			m.cookieMode = func() string { return "off" }
			calls = 0
			if _, err = m.Retry(s.Jobs[0].ID); job.State == "failed" && err == nil {
				waitJob(t, m)
				if calls != tc.calls {
					t.Fatal("retry changed mode")
				}
			}
		})
	}
}
func TestStandaloneClassifierBounds(t *testing.T) {
	if !cookieFallbackEligible("HTTP: 401 Unauthorized") || cookieFallbackEligible("No such file: 403.m3u8") || cookieAccessFailure("Could not find video formats") != "" {
		t.Fatal("classification boundary")
	}
}
func TestStandaloneCancelAndMissingAuto(t *testing.T) {
	m, d, _ := fixtureManager(t, "normal")
	m.cookieMode = func() string { return "auto" }
	m.detectBrowser = func() string { return "" }
	m.run = func(_ context.Context, _ string, _ []string, _ string, line func(string, string)) (uint32, error) {
		line("stderr", "HTTP Error 401")
		return 1, errors.New("401")
	}
	m.Start(Request{URL: "https://media.test/video", Directory: d})
	if !strings.Contains(waitJob(t, m).Error, "недоступны") {
		t.Fatal("missing browser")
	}
	m, d, _ = fixtureManager(t, "normal")
	m.cookieMode = func() string { return "edge" }
	started := make(chan struct{})
	calls := 0
	m.run = func(ctx context.Context, _ string, _ []string, _ string, line func(string, string)) (uint32, error) {
		calls++
		if calls == 1 {
			line("stderr", "HTTP Error 403")
			return 1, errors.New("403")
		}
		close(started)
		<-ctx.Done()
		return 1, ctx.Err()
	}
	s, _ := m.Start(Request{URL: "https://media.test/video", Directory: d})
	<-started
	m.Cancel(s.Jobs[0].ID)
	if waitJob(t, m).State != "cancelled" || calls != 2 {
		t.Fatal("cancel retry")
	}
	if entries, _ := os.ReadDir(m.paths.Temp); len(entries) != 0 {
		t.Fatal("temp retained")
	}
}
