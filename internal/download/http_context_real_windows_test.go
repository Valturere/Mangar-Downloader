//go:build windows

package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
)

func TestManagedProtectedHLSAndDASH(t *testing.T) {
	if os.Getenv("MANGAR_DOWNLOAD_INTEGRATION") != "1" {
		t.Skip("opt-in isolated managed tools")
	}
	_, tm, exes := managedFixture(t)
	for _, kind := range []string{"hls", "dash", "browser-hls", "browser-dash", "ffmpeg-hls", "ffmpeg-browser-hls"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			media := generateMedia(t, exes, root, "aac")
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			manifest := "media.m3u8"
			args := []string{"-nostdin", "-y", "-i", media, "-map", "0", "-c", "copy"}
			hls := strings.HasSuffix(kind, "hls")
			browser := strings.Contains(kind, "browser-")
			externalHLS := strings.HasPrefix(kind, "ffmpeg-")
			if hls {
				args = append(args, "-f", "hls", "-hls_time", "1", "-hls_list_size", "0", filepath.Join(root, manifest))
			} else {
				manifest = "manifest.mpd"
				args = append(args, "-f", "dash", filepath.Join(root, manifest))
			}
			if _, err := process.RunStreaming(ctx, exes["ffmpeg.exe"], args, root, nil); err != nil {
				t.Fatal(err)
			}
			if hls {
				os.WriteFile(filepath.Join(root, "master.m3u8"), []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=400000,CODECS=\"avc1.64000b,mp4a.40.2\"\nmedia.m3u8\n"), 0600)
				manifest = "master.m3u8"
			}
			const referer = "https://page.test/watch?token=HEADERSECRET"
			const origin = "https://page.test"
			var mu sync.Mutex
			seen := map[string]int{}
			denied := 0
			files := http.FileServer(http.Dir(root))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				cookie, ce := r.Cookie("session")
				if r.Header.Get("Referer") != referer || r.Header.Get("Origin") != origin || r.Header.Get("User-Agent") != "Mangar fixture" || (browser && (r.Header.Get("X-Playback") != "CUSTOMSECRET" || ce != nil || cookie.Value != "COOKIESECRET")) {
					denied++
					mu.Unlock()
					http.Error(w, "Forbidden", 403)
					return
				}
				seen[r.URL.Path]++
				mu.Unlock()
				files.ServeHTTP(w, r)
			}))
			defer server.Close()
			run := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
				if filepath.Base(exe) == "yt-dlp.exe" {
					// Diagnostic-only: normal production invocation stays native N8.
					// Cookie values here are synthetic; FFmpegFD puts them in child argv.
					if externalHLS {
						args = append([]string{"--downloader", "m3u8:ffmpeg", "--hls-use-mpegts"}, args...)
					}
					args = append([]string{"--proxy", "", "--retries", "0", "--fragment-retries", "0"}, args...)
				}
				return process.RunStreaming(ctx, exe, args, dir, line)
			}
			dest := filepath.Join(root, "output")
			os.MkdirAll(dest, 0700)
			m := New(Options{Paths: apppaths.At(filepath.Join(root, "app")), Tools: tm, Run: run})
			defer m.Shutdown(context.Background())
			wait := func() Job {
				m.mu.Lock()
				done := m.jobs[len(m.jobs)-1].done
				m.mu.Unlock()
				select {
				case <-done:
					return lastJob(m)
				case <-ctx.Done():
					m.Shutdown(context.Background())
					t.Fatal("protected media timeout")
					return Job{}
				}
			}
			s, err := m.Start(Request{URL: server.URL + "/" + manifest + "?token=SIGNEDSECRET", Directory: dest})
			if err != nil {
				t.Fatal(err)
			}
			failed := wait()
			if failed.State != "failed" || failed.Error != httpFailureMessage("HTTP 403") {
				t.Fatal(failed)
			}
			// Resolve a fixture-only profile through the same resolver used by admission.
			_, c := resolveHTTPProfile(server.URL+"/"+manifest, []httpProfile{{id: "fixture", host: "127.0.0.1", context: HTTPContext{Referer: referer, Origin: origin, UserAgent: "Mangar fixture"}}})
			request := Request{URL: server.URL + "/" + manifest + "?token=SIGNEDSECRET", Directory: dest, HTTPContext: c}
			if browser {
				c.Captured = true
				c.Headers = map[string]string{"X-Playback": "CUSTOMSECRET"}
				mediaType := "dash"
				if hls {
					mediaType = "hls"
				}
				request.Browser = &BrowserContext{MediaType: mediaType, PageTitle: "Видео из браузера", CapturedAt: time.Now(), ExactCookies: true, Cookies: []Cookie{{Name: "session", Value: "COOKIESECRET", Domain: "127.0.0.1", Path: "/", HostOnly: true, Session: true}}}
			}
			if _, err = m.Start(request); err != nil {
				t.Fatal(err)
			}
			completed := wait()
			if completed.State != "completed" {
				t.Fatal(completed)
			}
			validation, err := newLog(root, "validation")
			if err != nil {
				t.Fatal(err)
			}
			defer validation.close()
			if _, err = probe(ctx, run, exes, completed.FinalPath, validation, true); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			defer mu.Unlock()
			if denied == 0 || seen["/"+manifest] == 0 {
				t.Fatal("no protected manifest evidence")
			}
			if hls && seen["/media.m3u8"] == 0 {
				t.Fatal("child playlist not fetched")
			}
			fragments := 0
			for path, count := range seen {
				if strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".m4s") {
					fragments += count
				}
			}
			if fragments == 0 {
				t.Fatal("headers not proven on fragments")
			}
			if !hls {
				for _, stream := range []string{"0", "1"} {
					if seen["/init-stream"+stream+".m4s"] == 0 || seen["/chunk-stream"+stream+"-00001.m4s"] == 0 {
						t.Fatal("DASH initialization/media missing for stream", stream)
					}
				}
			}
			for _, id := range []string{s.Jobs[0].ExecutionID, completed.ExecutionID} {
				if _, err = os.Stat(filepath.Join(m.paths.Temp, id)); !os.IsNotExist(err) {
					t.Fatal("temp leak")
				}
				b, _ := os.ReadFile(filepath.Join(m.paths.Logs, "jobs", id+".log"))
				if strings.Contains(string(b), "SIGNEDSECRET") || strings.Contains(string(b), "HEADERSECRET") || strings.Contains(string(b), "Mangar fixture") || strings.Contains(string(b), "COOKIESECRET") || strings.Contains(string(b), "CUSTOMSECRET") {
					t.Fatal("HTTP context leaked")
				}
			}
			t.Logf("%s: baseline 403; protected manifest/child/fragments=%v; validated final MP4; temp clean", kind, seen)
		})
	}
}
