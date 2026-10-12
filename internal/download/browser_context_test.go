package download

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func browserFixture() *BrowserContext {
	return &BrowserContext{PageTitle: "Название страницы", MediaType: "hls", CapturedAt: time.Now(), Cookies: []Cookie{{Name: "s", Value: "COOKIE_SECRET&%", Domain: "cdn.test", Path: "/", HostOnly: true, Session: true, HTTPOnly: true}}}
}
func TestCookieJarFormatAndValidation(t *testing.T) {
	b := browserFixture()
	b.Cookies = append(b.Cookies, Cookie{Name: "long", Value: "value=1;unicode_日本語", Domain: ".cdn.test", Path: "/video", Secure: true, Expiration: 1900000000, PartitionKey: &PartitionKey{TopLevelSite: "https://page.test"}})
	valid, e := normalizeBrowserContext("https://cdn.test/video/master.m3u8", b)
	if e != nil {
		t.Fatal(e)
	}
	b.Cookies[0].Value = "changed"
	if valid.Cookies[0].Value == "changed" {
		t.Fatal("not frozen")
	}
	file, e := writeCookieJar(t.TempDir(), valid)
	if e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), "#HttpOnly_cdn.test\tFALSE\t/\tFALSE\t0\ts\tCOOKIE_SECRET&%") || !strings.Contains(string(data), ".cdn.test\tTRUE\t/video\tTRUE\t1900000000") {
		t.Fatal(string(data))
	}
	for _, v := range []string{"a\nb", "a\tb", "a\x00b"} {
		b = browserFixture()
		b.Cookies[0].Value = v
		if _, e = normalizeBrowserContext("https://cdn.test/video", b); e == nil {
			t.Fatal("injection")
		}
	}
}
func TestBrowserRetryCancelRemovePrivacy(t *testing.T) {
	m, dest, _ := fixtureManager(t, "exit")
	m.cookieMode = func() string { return "auto" }
	m.detectBrowser = func() string { t.Error("standalone browser detection for captured job"); return "chrome" }
	b := browserFixture()
	var jars []string
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) != "yt-dlp.exe" {
			return 1, errors.New("unexpected")
		}
		for i, a := range args {
			if a == "--cookies-from-browser" {
				t.Error("browser capture became standalone")
			}
			if a == "--cookies" {
				jars = append(jars, args[i+1])
				v, e := os.ReadFile(args[i+1])
				if e != nil || !strings.Contains(string(v), "COOKIE_SECRET") {
					t.Error("jar missing")
				}
			}
		}
		if strings.Contains(strings.Join(args, "|"), "COOKIE_SECRET") {
			t.Error("argv leaked")
		}
		line("stderr", "HTTP Error 403: Forbidden COOKIE_SECRET&% https://cdn.test/token-secret/master.m3u8")
		line("stderr", "Could not copy Chrome cookie database: DPAPI fixture")
		return 1, errors.New("COOKIE_SECRET&%")
	}
	s, e := m.Start(Request{URL: "https://cdn.test/master.m3u8?token=URL_SECRET", Directory: dest, Browser: b, HTTPContext: &HTTPContext{Captured: true, Headers: map[string]string{"X-Playback": "HEADER_SECRET"}}})
	if e != nil {
		t.Fatal(e)
	}
	first := waitJob(t, m)
	if !strings.Contains(first.Error, "несмотря на данные браузера") {
		t.Fatal(first)
	}
	b.Cookies[0].Value = "mutated"
	if _, e = m.Retry(s.Jobs[0].ID); e != nil {
		t.Fatal(e)
	}
	retry := waitJob(t, m)
	if strings.Contains(retry.Error, "cookies браузера") || !strings.Contains(retry.Error, "несмотря на данные браузера") {
		t.Fatal("captured retry received standalone cookie error")
	}
	if len(jars) != 2 || jars[0] == jars[1] {
		t.Fatal("no regenerated jar")
	}
	for _, path := range jars {
		if _, e = os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("jar leak")
		}
	}
	for _, j := range []Job{first, retry} {
		data, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log"))
		snapshot, _ := json.Marshal(m.Snapshot())
		for _, secret := range []string{"COOKIE_SECRET", "URL_SECRET", "HEADER_SECRET"} {
			if strings.Contains(string(data), secret) || strings.Contains(string(snapshot), secret) {
				t.Fatal("privacy", secret)
			}
		}
	}
	retained := m.jobs[0]
	if _, e = m.Remove(retained.view.ID); e != nil {
		t.Fatal(e)
	}
	if retained.request.Browser != nil || retained.request.HTTPContext != nil {
		t.Fatal("remove retained secrets")
	}
	// Cancellation must clean a jar already created for this execution.
	m, dest, _ = fixtureManager(t, "blocked")
	started := make(chan string, 1)
	m.run = func(ctx context.Context, _ string, args []string, _ string, _ func(string, string)) (uint32, error) {
		for i, a := range args {
			if a == "--cookies" {
				started <- args[i+1]
			}
		}
		<-ctx.Done()
		return 1, ctx.Err()
	}
	s, e = m.Start(Request{URL: "https://cdn.test/master.m3u8", Directory: dest, Browser: browserFixture()})
	if e != nil {
		t.Fatal(e)
	}
	jar := <-started
	m.Cancel(s.Jobs[0].ID)
	if waitJob(t, m).State != "cancelled" {
		t.Fatal("cancel")
	}
	if _, e = os.Stat(jar); !os.IsNotExist(e) {
		t.Fatal("cancel jar leaked")
	}
}
func TestBrowserTitleAndDRM(t *testing.T) {
	r := Request{URL: "https://cdn.test/index-f1-v1-a1.m3u8", Browser: browserFixture()}
	for _, title := range []string{"index-f1-v1-a1", "720.mp4:hls:manifest"} {
		if got, _ := browserTitle(title, r); got != r.Browser.PageTitle {
			t.Fatal(got)
		}
	}
	if got, _ := browserTitle("Настоящее название", r); got != "Настоящее название" {
		t.Fatal(got)
	}
	if browserFailure("This video is DRM protected", r) != "Видео защищено DRM и не поддерживается." {
		t.Fatal("DRM")
	}
	if browserFailure("HLS AES-128 encrypted", r) != "" {
		t.Fatal("AES blocked")
	}
}
