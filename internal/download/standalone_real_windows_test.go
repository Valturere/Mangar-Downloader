package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
)

func TestManagedStandaloneFirefoxAuth(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in native cookies loader; isolated controlled Firefox database")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	roaming := filepath.Join(root, "roaming")
	local := filepath.Join(root, "local")
	t.Setenv("APPDATA", roaming)
	t.Setenv("LOCALAPPDATA", local)
	profile := filepath.Join(roaming, "Mozilla", "Firefox", "Profiles", "controlled.default")
	os.MkdirAll(profile, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Real yt-dlp Firefox SQLite extraction, without a browser profile argument.
	python, err := exec.LookPath("python")
	if err != nil {
		t.Fatal("Python sqlite fixture dependency unavailable")
	}
	script := `import sqlite3,sys
c=sqlite3.connect(sys.argv[1])
c.execute('CREATE TABLE moz_cookies (host TEXT,name TEXT,value TEXT,path TEXT,expiry INTEGER,isSecure INTEGER)')
c.execute('PRAGMA user_version=17')
c.execute('INSERT INTO moz_cookies VALUES (?,?,?,?,?,?)',('127.0.0.1','session','CONTROLLED_COOKIE_SECRET','/',2000000000000,0))
c.commit()
`
	if _, err = exec.CommandContext(ctx, python, "-c", script, filepath.Join(profile, "cookies.sqlite")).CombinedOutput(); err != nil {
		t.Fatal("SQLite fixture creation failed")
	}
	media := generateMedia(t, exes, root, "aac")
	var denied, accepted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := r.Cookie("session")
		if e != nil || c.Value != "CONTROLLED_COOKIE_SECRET" {
			denied.Add(1)
			http.Error(w, "Login required", 401)
			return
		}
		accepted.Add(1)
		http.ServeFile(w, r, media)
	}))
	defer server.Close()
	attempts := 0
	withCookies := 0
	run := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			attempts++
			if slices.Contains(args, "--cookies-from-browser") {
				withCookies++
			}
			args = append([]string{"--proxy", "", "--retries", "0"}, args...)
		}
		return process.RunStreaming(ctx, exe, args, dir, line)
	}
	dest := filepath.Join(root, "Результат")
	os.MkdirAll(dest, 0700)
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run, CookieMode: func() string { return "auto" }})
	defer m.Shutdown(context.Background())
	if _, err = m.Start(Request{URL: server.URL + "/video.mkv", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	done := m.jobs[0].done
	m.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("auth download timeout")
	}
	job := lastJob(m)
	if job.State != "completed" || attempts != 2 || withCookies != 1 || denied.Load() == 0 || accepted.Load() == 0 {
		t.Fatalf("state=%s error=%s attempts=%d cookieAttempts=%d denied=%d accepted=%d", job.State, job.Error, attempts, withCookies, denied.Load(), accepted.Load())
	}
	log, _ := newLog(t.TempDir(), "probe")
	defer log.close()
	if _, err = probe(ctx, run, exes, job.FinalPath, log, true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", job.ExecutionID+".log"))
	if strings.Contains(string(data), "CONTROLLED_COOKIE_SECRET") || strings.Contains(string(data), profile) {
		t.Fatal("cookie/profile leaked")
	}
	if entries, _ := os.ReadDir(m.paths.Temp); len(entries) != 0 {
		t.Fatal("temp leaked")
	}
	t.Log("native Firefox SQLite loader: 401 → one cookies attempt → valid video+audio MP4; no cookie argv values; isolated profiles")
}

// A diagnostic smoke: a failed source is reported honestly, not treated as a
// proof of compatibility. MP4 success is probed if this provider begins working.
func TestManagedStandaloneVKDiagnostic(t *testing.T) {
	source := os.Getenv("MANGAR_VK_PAGE")
	if source == "" || os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in user supplied public page; no signed URL fixture")
	}
	_, tm, exes := managedFixture(t)
	root := t.TempDir()
	dest := filepath.Join(root, "output")
	os.MkdirAll(dest, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	attempts := 0
	cookieAttempts := 0
	unsupported := false
	run := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			attempts++
			if slices.Contains(args, "--cookies-from-browser") {
				cookieAttempts++
			}
		}
		return process.RunStreaming(ctx, exe, args, dir, func(stream, s string) {
			if strings.Contains(strings.ToLower(s), "unsupported url") {
				unsupported = true
			}
			if line != nil {
				line(stream, s)
			}
		})
	}
	m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run, CookieMode: func() string { return "auto" }})
	defer m.Shutdown(context.Background())
	if _, e := m.Start(Request{URL: source, Directory: dest}); e != nil {
		t.Fatal("source admission failed")
	}
	m.mu.Lock()
	done := m.jobs[0].done
	m.mu.Unlock()
	select {
	case <-done:
	case <-ctx.Done():
		m.Shutdown(context.Background())
		t.Fatal("source diagnostic timeout")
	}
	job := lastJob(m)
	if job.State == "completed" {
		log, _ := newLog(t.TempDir(), "probe")
		defer log.close()
		if _, e := probe(ctx, run, exes, job.FinalPath, log, true); e != nil {
			t.Fatal(e)
		}
	}
	if entries, _ := os.ReadDir(m.paths.Temp); len(entries) != 0 {
		t.Fatal("cleanup")
	}
	t.Logf("Standalone VK source page: state=%s unsupported_url=%t attempts=%d cookie_attempts=%d safe_error=%s", job.State, unsupported, attempts, cookieAttempts, job.Error)
}
