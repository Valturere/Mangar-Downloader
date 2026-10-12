package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserOneImpersonationFallback(t *testing.T) {
	for _, available := range []bool{true, false} {
		m, dest, _ := fixtureManager(t, "exit")
		normal, probe, fallback := 0, 0, 0
		m.run = func(_ context.Context, _ string, args []string, dir string, line func(string, string)) (uint32, error) {
			joined := strings.Join(args, "|")
			if strings.Contains(joined, "--list-impersonate-targets") {
				probe++
				if !available {
					return 1, errors.New("unavailable")
				}
				line("stdout", "Chrome-116 Windows-10 curl_cffi")
				line("stdout", "Chrome-145 Macos-26 curl_cffi")
				return 0, nil
			}
			if strings.Contains(joined, "--impersonate|chrome-116:windows-10") {
				fallback++
			} else {
				normal++
			}
			if !strings.Contains(joined, "X-Playback:EXACT") {
				t.Error("context lost")
			}
			if !strings.Contains(joined, "--cookies|") {
				t.Error("cookies lost")
			}
			line("stderr", "HTTP Error 403: Forbidden")
			return 1, errors.New("403")
		}
		_, e := m.Start(Request{URL: "https://cdn.test/master.m3u8", Directory: dest, Browser: browserFixture(), HTTPContext: &HTTPContext{Captured: true, Headers: map[string]string{"X-Playback": "EXACT"}}})
		if e != nil {
			t.Fatal(e)
		}
		j := waitJob(t, m)
		if normal != 1 || probe != 1 || fallback != boolInt(available) {
			t.Fatal(normal, probe, fallback)
		}
		if _, e = os.Stat(filepath.Join(m.paths.Temp, j.ExecutionID)); !os.IsNotExist(e) {
			t.Fatal("temp")
		}
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
