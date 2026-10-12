package browserbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/download"
	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/tools"
	"golang.org/x/sys/windows"
)

// Developer-only native receiver. Every attempt needs a fresh, explicitly selected
// browser capture. No production option, automatic N sweep or saved payload.
func TestRuntimeBrowserHLSDiagnostic(t *testing.T) {
	root, page := os.Getenv("MANGAR_HLS_DIAGNOSTIC_ROOT"), os.Getenv("MANGAR_HLS_DIAGNOSTIC_PAGE")
	if root == "" || page == "" {
		t.Skip("opt-in fresh user browser HLS diagnostic")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("diagnostic root must be absolute")
	}
	paths := testfixture.Tools(t)
	tm := tools.New(tools.Options{Paths: paths})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	if err := tm.Detect(ctx); err != nil {
		t.Fatal(err)
	}
	id, err := UserIdentity(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		t.Fatal("identity unavailable")
	}
	claim, first, err := Claim(id)
	if claim != 0 {
		defer windows.CloseHandle(claim)
	}
	if err != nil || !first {
		t.Fatal("close Mangar before diagnostic receiver")
	}
	requests := make(chan Message, 1)
	var busy atomic.Bool
	server, err := Listen(id, func(_ context.Context, m Message) Reply {
		if m.Type != "media_capture" {
			return Failure("invalid")
		}
		if m.Capture.Browser.PageURL != page {
			reply := Failure("invalid")
			reply.Message = "Диагностика ожидает исходную страницу. Вернитесь к выбранной серии"
			return reply
		}
		if m.Capture.Browser.MediaType != "hls" {
			reply := Failure("invalid")
			reply.Message = "Для этой диагностики выберите HLS (.m3u8)"
			return reply
		}
		if time.Since(m.Capture.Browser.CapturedAt) > 90*time.Second {
			return Failure("expired")
		}
		if !busy.CompareAndSwap(false, true) {
			return Failure("busy")
		}
		requests <- m
		return Success()
	})
	if err != nil {
		t.Fatal("diagnostic pipe unavailable")
	}
	defer server.Close()
	if err := os.MkdirAll(filepath.Join(root, "output"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Log("READY: штатное расширение → helper → SID pipe; изолированный engine; strategy.txt selects one attempt")
	for {
		select {
		case m := <-requests:
			strategy, err := os.ReadFile(filepath.Join(root, "strategy.txt"))
			if err != nil {
				t.Fatal("strategy file unavailable")
			}
			s := strings.TrimSpace(string(strategy))
			if !slices.Contains([]string{"32", "16", "8", "4", "2", "1", "ffmpeg"}, s) {
				t.Fatal("invalid diagnostic strategy")
			}
			result := diagnoseHLS(ctx, tm, root, s, m.Capture)
			b, _ := json.Marshal(result)
			f, err := os.OpenFile(filepath.Join(root, "results.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			_, writeErr := f.Write(append(b, '\n'))
			closeErr := f.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatal("diagnostic report write failed")
			}
			t.Log(string(b))
			busy.Store(false)
		case <-time.After(time.Second):
			if _, err := os.Stat(filepath.Join(root, "stop")); err == nil {
				return
			}
		case <-ctx.Done():
			t.Fatal("diagnostic receiver deadline")
		}
	}
}

func diagnoseHLS(parent context.Context, tm *tools.Manager, root, strategy string, c *Capture) map[string]any {
	result := map[string]any{"strategy": strategy, "fresh_capture": true, "cookies": len(c.Browser.Cookies), "header_names": diagnosticHeaderNames(c.HTTP)}
	// FFmpegFD in managed yt-dlp passes cookies in child argv. Do not run real
	// private cookies through that path, even in a developer experiment.
	if strategy == "ffmpeg" && len(c.Browser.Cookies) != 0 {
		result["result"] = "blocked_cookie_argv"
		return result
	}
	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
	defer cancel()
	diagnosticCtx := ctx
	guard := diagnosticAccessGuard{cancel: cancel}
	result["manifest"] = inspectHLS(ctx, c)
	started := time.Now()
	var missing, skipped, requests, replies, statuses4xx int
	var native, liveWarning bool
	var speed float64
	fragmentPattern := regexp.MustCompile(`\.part-Frag([0-9]+)`)
	var existed atomic.Bool
	var created syncFragmentNames
	runner := func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		// Link every engine attempt/probe to the diagnostic deadline and access
		// guard. Cancelling the experiment must also stop an internal auth retry.
		ctx, stopRun := context.WithCancel(ctx)
		unlink := context.AfterFunc(diagnosticCtx, stopRun)
		defer unlink()
		defer stopRun()
		if diagnosticCtx.Err() != nil {
			return 0, diagnosticCtx.Err()
		}
		if filepath.Base(exe) != "yt-dlp.exe" {
			return process.RunStreaming(ctx, exe, args, dir, line)
		}
		args = slices.Clone(args)
		for i := range args {
			if args[i] == "-N" && i+1 < len(args) && strategy != "ffmpeg" {
				args[i+1] = strategy
			}
		}
		extra := []string{"--no-colors", "--verbose", "--print-traffic", "--downloader", "m3u8:native"}
		if strategy == "ffmpeg" {
			extra = []string{"--downloader", "m3u8:ffmpeg", "--hls-use-mpegts"}
		}
		args = append(extra, args...)
		watchCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				entries, _ := os.ReadDir(dir)
				for _, entry := range entries {
					if fragmentPattern.MatchString(entry.Name()) {
						created.add(entry.Name())
					}
				}
				select {
				case <-watchCtx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		}()
		code, err := process.RunStreaming(ctx, exe, args, dir, func(stream, text string) {
			// http.client traffic contains request paths and credentials even without
			// complete URLs. Classify in RAM; never forward it to the technical log.
			trim := strings.TrimSpace(text)
			if strings.HasPrefix(trim, "send:") {
				requests++
				return
			}
			if strings.HasPrefix(trim, "reply:") {
				replies++
				if strings.Contains(trim, " 4") {
					statuses4xx++
				}
				return
			}
			if diagnosticRawTraffic(trim) {
				return
			}
			guard.observe(trim)
			native = native || strings.HasPrefix(trim, "[hlsnative]")
			liveWarning = liveWarning || strings.Contains(text, "Live HLS streams are not supported by the native downloader")
			if strings.Contains(text, "Skipping fragment") {
				skipped++
			}
			if match := fragmentPattern.FindStringSubmatch(text); len(match) > 1 && strings.Contains(text, "No such file or directory") {
				missing, _ = strconv.Atoi(match[1])
				existed.Store(created.has("media.mp4.part-Frag" + match[1]))
			}
			if strings.HasPrefix(text, "MANGAR_PROGRESS:") {
				var p struct {
					Speed *float64 `json:"speed"`
				}
				if json.Unmarshal([]byte(strings.TrimPrefix(text, "MANGAR_PROGRESS:")), &p) == nil && p.Speed != nil {
					speed = *p.Speed
				}
			}
			line(stream, text)
		})
		stop()
		<-done
		return code, err
	}
	paths := apppaths.At(filepath.Join(root, "app"))
	manager := download.New(download.Options{Paths: paths, Tools: tm, Run: runner})
	defer manager.Shutdown(context.Background())
	// Repeats must not fail only because a previous successful MP4 has the same
	// title. Keep each experiment's output without deleting or overwriting it.
	outputDir, err := os.MkdirTemp(filepath.Join(root, "output"), "attempt-")
	if err != nil {
		result["result"] = "output_directory_failed"
		return result
	}
	_, err = manager.Start(download.Request{URL: c.URL, Directory: outputDir, HTTPContext: &c.HTTP, Browser: &c.Browser})
	if err != nil {
		result["result"] = "admission_failed"
		return result
	}
	for {
		s := manager.Snapshot()
		if len(s.Jobs) == 1 && !s.Jobs[0].Active {
			j := s.Jobs[0]
			result["result"], result["execution_id"] = j.State, j.ExecutionID
			if guard.status != 0 {
				result["network_guard_stop"] = guard.status
			}
			result["elapsed_seconds"] = time.Since(started).Seconds()
			result["last_speed_bytes_sec"] = speed
			result["native_hls"], result["live_warning"] = native, liveWarning
			result["missing_fragment"], result["skipped_fragments"] = missing, skipped
			result["missing_file_seen_before"] = existed.Load()
			result["http_requests"], result["http_replies"], result["http_4xx"] = requests, replies, statuses4xx
			_, err := os.Stat(filepath.Join(paths.Temp, j.ExecutionID))
			result["temp_cookie_jar_clean"] = os.IsNotExist(err)
			if j.FinalPath != "" {
				// Engine finalization probes MP4; explicit acceptance requires A+V.
				lease, err := tm.Acquire(ctx, tools.FFmpeg)
				if err == nil {
					var output strings.Builder
					_, err = process.RunStreaming(ctx, lease.Paths["ffprobe.exe"], []string{"-v", "error", "-show_entries", "stream=codec_type:format=format_name", "-of", "json", j.FinalPath}, root, func(_, line string) {
						if output.Len() < 65536 {
							output.WriteString(line)
						}
					})
					lease.Close()
					var probe struct {
						Streams []struct {
							Type string `json:"codec_type"`
						} `json:"streams"`
						Format struct {
							Name string `json:"format_name"`
						} `json:"format"`
					}
					if err == nil && json.Unmarshal([]byte(output.String()), &probe) == nil {
						for _, stream := range probe.Streams {
							result["ffprobe_"+stream.Type] = true
						}
						result["ffprobe_mp4"] = strings.Contains(probe.Format.Name, "mp4")
					}
				}
			}
			return result
		}
		select {
		case <-ctx.Done():
			manager.Shutdown(context.Background())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Python HTTP debug prints can split "send:" and the following bytes literal
// across stdout/stderr. curl_cffi verbose traffic uses >, < and * prefixes.
// Suppress whole records, rather than trying to redact unknown request tokens.
var diagnosticHTTPRequest = regexp.MustCompile(`(?:^|[\s'">])(?:GET|HEAD|POST|OPTIONS)\s+/\S+\s+HTTP/[123](?:\.[0-9]+)?`)

func diagnosticRawTraffic(line string) bool {
	for _, prefix := range []string{"send:", "reply:", "header:", "[debug]", "b'", `b"`, "> ", "< ", "* "} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return diagnosticHTTPRequest.MatchString(line) || (strings.Contains(line, "[HTTP/2]") && strings.Contains(line, "[:path:"))
}

func TestDiagnosticRawTraffic(t *testing.T) {
	for _, raw := range []string{
		"send: ", "reply: 'HTTP/1.1 403 Forbidden'", "header: Cookie: SECRET",
		"[debug] command SECRET", "b'GET /SIGNEDSECRET/master.m3u8 HTTP/1.1'",
		`b"GET /SIGNEDSECRET/seg.ts HTTP/1.1"`, "> GET /SIGNEDSECRET/master.m3u8 HTTP/2",
		"> Cookie: SECRET", "< Set-Cookie: SECRET", "* [HTTP/2] [:path: /SIGNEDSECRET]",
		"interleaved progress b'GET /SIGNEDSECRET/master.m3u8 HTTP/1.1'",
	} {
		if !diagnosticRawTraffic(raw) {
			t.Fatal("raw HTTP traffic would reach technical log")
		}
	}
	for _, safe := range []string{"MANGAR_META:{\"id\":\"master\"}", "MANGAR_PROGRESS:{}", "[hlsnative] Total fragments: 120", "[download] Got error: HTTP Error 403: Forbidden", "ERROR: No such file or directory: media.mp4.part-Frag20"} {
		if diagnosticRawTraffic(safe) {
			t.Fatal("safe diagnostic or engine marker was suppressed")
		}
	}
}

// Diagnostic-only: do not let ten retries per fragment multiply a CDN refusal
// into hundreds of requests. This is not a production HLS classifier/fallback.
type diagnosticAccessGuard struct {
	cancel   context.CancelFunc
	failures int
	status   int
}

var diagnosticDeniedFragment = regexp.MustCompile(`^\[download\] Got error: HTTP Error (403|429):`)

func (g *diagnosticAccessGuard) observe(line string) {
	match := diagnosticDeniedFragment.FindStringSubmatch(line)
	if len(match) != 2 || g.status != 0 {
		return
	}
	g.failures++
	if g.failures == 3 {
		g.status, _ = strconv.Atoi(match[1])
		g.cancel()
	}
}

func TestDiagnosticAccessGuard(t *testing.T) {
	for _, status := range []string{"403", "429"} {
		t.Run(status, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			guard := diagnosticAccessGuard{cancel: cancel}
			for _, unrelated := range []string{"HTTP Error 403", "[debug] HTTP Error 403: Forbidden", "[download] Got error: HTTP Error 404: Not Found", "MANGAR_PROGRESS:{\"downloaded_bytes\":403}"} {
				guard.observe(unrelated)
			}
			for i := 0; i < 3; i++ {
				if ctx.Err() != nil {
					t.Fatal("stopped before three explicit denied fragment errors")
				}
				guard.observe("[download] Got error: HTTP Error " + status + ": denied. Retrying fragment 21 (1/10)...")
			}
			if ctx.Err() != context.Canceled || strconv.Itoa(guard.status) != status || guard.failures != 3 {
				t.Fatal("denied fragment guard did not cancel diagnostic context")
			}
			guard.observe("[download] Got error: HTTP Error " + status + ": denied")
			if guard.failures != 3 {
				t.Fatal("guard retriggered after cancellation")
			}
		})
	}
}

type syncFragmentNames struct{ names sync.Map }

func (s *syncFragmentNames) add(name string) {
	s.names.Store(name, true)
}
func (s *syncFragmentNames) has(name string) bool {
	_, found := s.names.Load(name)
	return found
}

func diagnosticHeaderNames(h download.HTTPContext) []string {
	names := []string{}
	if h.Referer != "" {
		names = append(names, "Referer")
	}
	if h.Origin != "" {
		names = append(names, "Origin")
	}
	if h.UserAgent != "" {
		names = append(names, "User-Agent")
	}
	for name := range h.Headers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func diagnosticCookieJar(c *Capture) *cookiejar.Jar {
	jar, _ := cookiejar.New(nil)
	u, _ := url.Parse(c.URL)
	for _, cookie := range c.Browser.Cookies {
		domain := cookie.Domain
		if cookie.HostOnly {
			// A Domain attribute broadens scope to subdomains in Go's cookiejar.
			domain = ""
		}
		value := &http.Cookie{Name: cookie.Name, Value: cookie.Value, Domain: domain, Path: cookie.Path, Secure: cookie.Secure, HttpOnly: cookie.HTTPOnly}
		if !cookie.Session {
			value.Expires = time.Unix(int64(cookie.Expiration), 0)
		}
		jar.SetCookies(u, []*http.Cookie{value})
	}
	return jar
}

func TestDiagnosticCookieScope(t *testing.T) {
	capture := &Capture{URL: "https://cdn.example/video/master.m3u8", Browser: download.BrowserContext{Cookies: []download.Cookie{
		{Name: "host", Value: "HOSTSECRET", Domain: "cdn.example", Path: "/video/", HostOnly: true, Secure: true, Session: true},
		{Name: "domain", Value: "DOMAINSECRET", Domain: ".cdn.example", Path: "/video/", Secure: true, Session: true},
		{Name: "expired", Value: "EXPIREDSECRET", Domain: "cdn.example", Path: "/video/", HostOnly: true, Secure: true, Expiration: 1},
	}}}
	jar := diagnosticCookieJar(capture)
	for _, tc := range []struct {
		url  string
		want []string
	}{
		{"https://cdn.example/video/media.m3u8", []string{"domain", "host"}},
		{"https://sub.cdn.example/video/media.m3u8", []string{"domain"}},
		{"https://other.example/video/media.m3u8", nil},
		{"https://cdn.example/other/media.m3u8", nil},
		{"http://cdn.example/video/media.m3u8", nil},
	} {
		u, _ := url.Parse(tc.url)
		var names []string
		for _, cookie := range jar.Cookies(u) {
			names = append(names, cookie.Name)
		}
		slices.Sort(names)
		if !slices.Equal(names, tc.want) {
			t.Fatalf("cookie scope mismatch for synthetic URL %s: names=%v", tc.url, names)
		}
	}
}

// Only structural facts are persisted. Playlists/segment URLs remain in RAM.
func inspectHLS(ctx context.Context, c *Capture) map[string]any {
	jar := diagnosticCookieJar(c)
	u, _ := url.Parse(c.URL)
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(raw string) (string, int) {
		r, err := http.NewRequestWithContext(ctx, "GET", raw, nil)
		if err != nil {
			return "", 0
		}
		if c.HTTP.Referer != "" {
			r.Header.Set("Referer", c.HTTP.Referer)
		}
		if c.HTTP.Origin != "" {
			r.Header.Set("Origin", c.HTTP.Origin)
		}
		if c.HTTP.UserAgent != "" {
			r.Header.Set("User-Agent", c.HTTP.UserAgent)
		}
		for name, value := range c.HTTP.Headers {
			r.Header.Set(name, value)
		}
		response, err := client.Do(r)
		if err != nil {
			return "", 0
		}
		defer response.Body.Close()
		b, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		if err != nil || len(b) > 2<<20 {
			return "", response.StatusCode
		}
		return string(b), response.StatusCode
	}
	text, status := get(c.URL)
	facts := map[string]any{"http_status": status}
	if status != 200 || !strings.HasPrefix(text, "#EXTM3U") {
		return facts
	}
	master := strings.Contains(text, "#EXT-X-STREAM-INF:")
	facts["master"] = master
	mediaURL := c.URL
	if master {
		// Observe a variant, without changing the selected capture sent to engine.
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				ref, err := url.Parse(line)
				if err == nil {
					mediaURL = u.ResolveReference(ref).String()
					text, status = get(mediaURL)
				}
				break
			}
		}
		facts["variant_http_status"] = status
	}
	if status != 200 {
		return facts
	}
	field := func(text, name string) string {
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, name+":") {
				return strings.TrimSpace(strings.TrimPrefix(line, name+":"))
			}
		}
		return ""
	}
	sequence := field(text, "#EXT-X-MEDIA-SEQUENCE")
	facts["endlist"] = strings.Contains(text, "#EXT-X-ENDLIST")
	if number, err := strconv.ParseUint(sequence, 10, 64); err == nil {
		facts["media_sequence"] = number
	}
	if kind := field(text, "#EXT-X-PLAYLIST-TYPE"); kind == "VOD" || kind == "EVENT" {
		facts["playlist_type"] = kind
	}
	if number, err := strconv.ParseUint(field(text, "#EXT-X-TARGETDURATION"), 10, 64); err == nil {
		facts["targetduration"] = number
	}
	facts["discontinuities"] = strings.Count(text, "#EXT-X-DISCONTINUITY\n")
	var first string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			first = line
			break
		}
	}
	select {
	case <-ctx.Done():
		return facts
	case <-time.After(3 * time.Second):
	}
	next, status := get(mediaURL)
	facts["second_http_status"] = status
	if status == 200 {
		facts["media_sequence_changes"] = sequence != field(next, "#EXT-X-MEDIA-SEQUENCE")
		facts["first_fragment_still_listed"] = first != "" && strings.Contains(next, first)
	}
	return facts
}
