package download

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHTTPContextArgvAndSecurity(t *testing.T) {
	paths := map[string]string{"deno.exe": "deno", "ffmpeg.exe": "ffmpeg"}
	baseline := ytdlpArgs(paths, "temp", "https://example.test/media.mpd")
	if !reflect.DeepEqual(baseline, ytdlpArgs(paths, "temp", "https://example.test/media.mpd", nil)) {
		t.Fatal("no-context argv changed")
	}
	for _, input := range []*HTTPContext{
		{Referer: "https://page.test/watch?a=1&b=2"}, {Origin: "https://page.test"}, {UserAgent: "Agent 1"},
		{Referer: "https://page.test/", Origin: "https://page.test", UserAgent: "Agent 1", Headers: map[string]string{"accept-language": "ru"}},
	} {
		c, err := normalizeHTTPContext(input)
		if err != nil {
			t.Fatal(err)
		}
		args := ytdlpArgs(paths, "temp", "https://example.test/media.mpd", c)
		if args[len(args)-2] != "--" || args[len(args)-1] != "https://example.test/media.mpd" {
			t.Fatal(args)
		}
		for _, name := range c.headerNames() {
			if !strings.Contains(strings.Join(args, "|"), "--add-headers|"+name+":"+c.headerValue(name)) {
				t.Fatal(args)
			}
		}
	}
	for _, input := range []*HTTPContext{
		{Referer: "https://page.test/\r\nCookie:x"}, {Origin: "https://page.test/path"}, {UserAgent: "agent\nHost:x"}, {UserAgent: "a\x00b"},
		{Headers: map[string]string{"Accept\r\nX": "yes"}}, {Headers: map[string]string{"Accept": "x\ry"}},
		{Headers: map[string]string{"Accept": "x", "accept": "y"}},
	} {
		if _, err := normalizeHTTPContext(input); err == nil {
			t.Fatal("accepted invalid context")
		}
	}
	for _, name := range []string{"Host", "Content-Length", "Connection", "Proxy-Authorization", "Proxy-Connection", "Cookie", "Authorization", "Transfer-Encoding", "Referer", "Origin", "User-Agent", "X-Secret"} {
		if _, err := normalizeHTTPContext(&HTTPContext{Headers: map[string]string{name: "secret"}}); err == nil {
			t.Fatal("accepted forbidden header", name)
		}
	}
}
func TestHTTPProfileExactSubdomainAndUnknown(t *testing.T) {
	profiles := []httpProfile{{id: "fixture", host: "cdn.test", context: HTTPContext{Origin: "https://page.test"}}}
	for _, host := range []string{"cdn.test", "a.cdn.test", "A.CDN.TEST."} {
		id, c := resolveHTTPProfile("https://"+host+"/manifest.mpd", profiles)
		if id != "fixture" || c == nil {
			t.Fatal(host)
		}
	}
	for _, host := range []string{"other.test", "fakecdn.test", "cdn.test.evil.test"} {
		id, c := resolveHTTPProfile("https://"+host+"/index.m3u8", profiles)
		if id != "" || c != nil {
			t.Fatal(host)
		}
	}
	if id, c := resolveHTTPProfile("https://unknown.test/video.mp4", httpProfiles); id != "" || c != nil {
		t.Fatal("unknown has provider")
	}
}
func TestHTTPContextFrozenAndRetryRetained(t *testing.T) {
	m, dest, _ := fixtureManager(t, "exit")
	supplied := &HTTPContext{Referer: "https://page.test/?token=HEADERSECRET", Headers: map[string]string{"Accept-Language": "ru"}}
	s, err := m.Start(Request{URL: "https://cdn.test/index.m3u8", Directory: dest, HTTPContext: supplied})
	if err != nil {
		t.Fatal(err)
	}
	supplied.Referer = "https://changed.test/"
	supplied.Headers["Accept-Language"] = "en"
	first := waitJob(t, m)
	var captured []string
	m.run = func(_ context.Context, _ string, args []string, _ string, line func(string, string)) (uint32, error) {
		captured = append([]string{}, args...)
		line("stderr", "HTTP Error 403: Forbidden https://cdn.test/SIGNEDSECRET/index.m3u8")
		line("stderr", "Referer:https://page.test/?token=HEADERSECRET")
		return 1, errors.New("exit 1")
	}
	_, err = m.Retry(s.Jobs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	retry := waitJob(t, m)
	if first.ExecutionID == retry.ExecutionID || first.ID != retry.ID || retry.Attempt != 2 {
		t.Fatal("retry identity")
	}
	if !strings.Contains(strings.Join(captured, "|"), "Referer:https://page.test/?token=HEADERSECRET") || !strings.Contains(strings.Join(captured, "|"), "Accept-Language:ru") {
		t.Fatal("retry lost/fetched mutated context")
	}
	if retry.Error != httpFailureMessage("HTTP 403") {
		t.Fatal(retry)
	}
	b, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", retry.ExecutionID+".log"))
	if strings.Contains(string(b), "SIGNEDSECRET") || strings.Contains(string(b), "HEADERSECRET") {
		t.Fatal("URL/header leak")
	}
}

func TestHTTPProfilePathAndExternalContextSeparation(t *testing.T) {
	old := httpProfiles
	httpProfiles = []httpProfile{{id: "fixture", host: "cdn.test", pathSuffix: ".m3u8", context: HTTPContext{Referer: "https://trusted.test/"}}}
	t.Cleanup(func() { httpProfiles = old })
	known, err := prepareHTTPRequest(Request{URL: "https://a.cdn.test/path/MASTER.M3U8?token=fixture"})
	if err != nil || known.httpProfile != "fixture" || known.HTTPContext.Referer != "https://trusted.test/" {
		t.Fatal("profile path match", err)
	}
	other, err := prepareHTTPRequest(Request{URL: "https://a.cdn.test/path/manifest.mpd"})
	if err != nil || other.httpProfile != "" || other.HTTPContext != nil {
		t.Fatal("unmatched path acquired context", err)
	}
	external, err := prepareHTTPRequest(Request{URL: known.URL, httpProfile: "spoof", HTTPContext: &HTTPContext{Origin: "https://external.test"}})
	if err != nil || external.httpProfile != "" || external.HTTPContext.Referer != "" || external.HTTPContext.Origin != "https://external.test" {
		t.Fatal("external and trusted contexts mixed", err)
	}
}
func TestBatchResolvesHTTPContext(t *testing.T) {
	old := httpProfiles
	httpProfiles = []httpProfile{{id: "fixture", host: "cdn.test", context: HTTPContext{Origin: "https://page.test"}}}
	t.Cleanup(func() { httpProfiles = old })
	m, dest, _ := fixtureManager(t, "blocked")
	txt := filepath.Join(t.TempDir(), "urls.txt")
	os.WriteFile(txt, []byte("https://cdn.test/index.m3u8\nhttps://unknown.test/index.mpd"), 0600)
	if _, err := m.ImportTXT(txt, dest); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.jobs[0].request.httpProfile != "fixture" || m.jobs[0].request.HTTPContext.Origin != "https://page.test" || m.jobs[1].request.HTTPContext != nil {
		t.Fatal("batch profile resolution")
	}
}
func TestHTTPFailureAcrossMetadataAndFragments(t *testing.T) {
	for _, phase := range []string{"manifest", "fragment"} {
		for _, status := range []string{"401", "403", "404", "timeout", "network", "generic"} {
			t.Run(phase+status, func(t *testing.T) {
				m, dest, _ := fixtureManager(t, "exit")
				raw := "ERROR: generic extraction failed"
				if status == "timeout" {
					raw = "ERROR: timed out"
				} else if status == "network" {
					raw = "ERROR: getaddrinfo failed"
				} else if status != "generic" {
					raw = "ERROR: Unable to download: HTTP Error " + status + ": Forbidden"
				}
				m.run = func(_ context.Context, _ string, _ []string, _ string, line func(string, string)) (uint32, error) {
					if phase == "fragment" {
						line("stdout", metaPrefix+`{"id":"test"}`)
						line("stderr", progressPrefix+`{"downloaded_bytes":1}`)
					}
					line("stderr", raw+" https://cdn.test/SIGNEDSECRET?a=secret")
					return 1, errors.New("exit 1")
				}
				m.Start(Request{URL: "https://cdn.test/index.m3u8", Directory: dest})
				j := waitJob(t, m)
				want := httpFailureMessage(raw)
				if want == "" {
					if phase == "manifest" {
						want = "Не удалось получить информацию о видео"
					} else {
						want = "Ошибка загрузки"
					}
				}
				if j.Error != want || j.State != "failed" {
					t.Fatal(j)
				}
				b, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", j.ExecutionID+".log"))
				if !strings.Contains(string(b), raw) || strings.Contains(string(b), "SIGNEDSECRET") {
					t.Fatal("stderr retention/redaction")
				}
			})
		}
	}
}
