package fragmentdiag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/download"
)

func fixture(raw string) *Capture {
	return &Capture{Version: 1, Type: "fragment_diagnostic", CapturedAt: time.Now(), RequestID: "one", TabID: 1, FrameID: 2, Method: "GET", URL: raw, Headers: []Header{{"Referer", "https://player.test/watch"}, {"Origin", "https://player.test"}, {"User-Agent", "fixture-browser"}}, Response: Response{Status: 200}}
}
func TestURLAndHeaderStructuralComparison(t *testing.T) {
	d := compareURL("https://cdn.test/a%2Fb.ts?token=FIXTURE&token=SECOND&empty=", "https://cdn.test/a/b.ts?token=OTHER")
	if !d.PathEqual || d.RawPathEqual || d.QueryNamesEqual || d.QueryValuesEqual || !d.DuplicatesBrowser || d.QueryCountBrowser != 3 || len(d.MissingQueryNames) != 1 || d.MissingQueryNames[0] != "empty" {
		t.Fatal("structural comparison incorrect")
	}
	d = compareURL("https://cdn.test/a?x=%41", "https://cdn.test/a?x=A")
	if !d.QueryValuesEqual || d.RawQueryEqual {
		t.Fatal("encoding distinction lost")
	}
	d = compareURL("https://cdn.test/a?odd%20one=FIXTURE", "https://cdn.test/a?odd%20two=FIXTURE")
	if d.QueryNamesEqual || len(d.MissingQueryNames) != 1 || d.MissingQueryNames[0] != "[nonstandard]" {
		t.Fatal("safe name placeholder collapsed actual names")
	}
	a, b := http.Header{"Referer": {"REF_FIXTURE"}, "Range": {"bytes=0-1"}, "Cookie": {"COOKIE_FIXTURE"}}, http.Header{"Origin": {"ORIGIN_FIXTURE"}}
	h := compareHeaders(a, b)
	encoded, _ := json.Marshal(h)
	for _, secret := range []string{"REF_FIXTURE", "COOKIE_FIXTURE", "ORIGIN_FIXTURE", "bytes=0-1"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("report leaked value")
		}
	}
	if h.RefererEqual || h.RangeEqual || !h.CookieBrowser || !h.RangeBrowser || h.RangeReplay || len(h.BrowserOnly) != 3 || len(h.ReplayOnly) != 1 {
		t.Fatal("header comparison")
	}
}
func TestReplayBudgetRedirectPrivacyAndManifestResolution(t *testing.T) {
	var requests atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path == "/variant.m3u8" {
			if r.URL.Query().Get("ambiguous") != "" {
				w.Write([]byte("#EXTM3U\n#EXTINF:12,\n1.ts?x=ONE\n#EXTINF:12,\n1.ts?x=TWO\n"))
				return
			}
			w.Write([]byte("#EXTM3U\n#EXTINF:12,\n1.ts\n#EXT-X-ENDLIST\n"))
			return
		}
		w.Header().Set("Set-Cookie", "RESPONSE_FIXTURE")
		w.Write([]byte("media"))
	}))
	defer s.Close()
	c := fixture(s.URL + "/1.ts?token=SIGNED_FIXTURE")
	c.Manifests = []Manifest{{URL: s.URL + "/variant.m3u8?token=SIGNED_FIXTURE", HTTP: download.HTTPContext{}}}
	r := Run(context.Background(), c)
	if r.Exact.Status != 200 || !r.Exact.BodyStart || r.FragmentReplays != 1 || requests.Load() != 2 || !r.ManifestMatched || r.ResolvedURL.QueryNamesEqual || r.ResolvedURL.QueryValuesEqual || !r.VariantQueryPresent || r.SegmentURIAbsolute {
		t.Fatal("replay / manifest resolution")
	}
	if c.URL != "" || c.Headers != nil {
		t.Fatal("raw capture retained")
	}
	other := fixture(s.URL + "/1.ts?x=UNRELATED")
	other.Manifests = []Manifest{{URL: s.URL + "/variant.m3u8?ambiguous=true"}}
	if Run(context.Background(), other).ManifestMatched {
		t.Fatal("ambiguous media URI chosen")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "FIXTURE") || strings.Contains(string(b), "1.ts") {
		t.Fatal("report leaked raw context")
	}
	var redirects atomic.Int32
	rs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		redirects.Add(1)
		http.Redirect(w, req, "/again?token=REDIRECT_FIXTURE", 302)
	}))
	defer rs.Close()
	r = Run(context.Background(), fixture(rs.URL+"/one.ts"))
	if redirects.Load() != 1 || r.FragmentReplays != 1 || r.Exact.Status != 302 || r.Ytdlp.SentRequests != 0 {
		t.Fatal("redirect expanded replay count")
	}
}
func TestTTLInvalidAuthAndCancellationMakeNoFollowingRequests(t *testing.T) {
	var count atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { count.Add(1); <-r.Context().Done() }))
	defer s.Close()
	c := fixture(s.URL + "/1.ts")
	c.CapturedAt = time.Now().Add(-TTL - time.Second)
	if Run(context.Background(), c).Result != "invalid" {
		t.Fatal("expired accepted")
	}
	c = fixture(s.URL + "/1.ts")
	c.Headers = append(c.Headers, Header{"Authorization", "AUTH_FIXTURE"})
	r := Run(context.Background(), c)
	if r.Result != "blocked_header_boundary" || !r.ProductionHeaders.AuthorizationObserved || r.FragmentReplays != 0 {
		t.Fatal("auth boundary bypassed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if Run(ctx, fixture(s.URL+"/1.ts")).Result != "cancelled" || count.Load() != 0 {
		t.Fatal("request after cancellation")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	c = fixture(s.URL + "/1.ts")
	r = Run(ctx, c)
	if count.Load() != 1 || r.Ytdlp.SentRequests != 0 || c.URL != "" {
		t.Fatal("active cancellation failed")
	}
}
func TestTrafficParsingIsTransientAndStructural(t *testing.T) {
	line := `send: b'GET /1.ts?token=SIGNED_FIXTURE HTTP/1.1\r\nHost: cdn.test\r\nReferer: https://player.test/\r\nCookie: s=COOKIE_FIXTURE\r\n\r\n'`
	raw, h, ok := trafficRequest(line, "https://cdn.test/1.ts?token=SIGNED_FIXTURE")
	if !ok || !compareURL(raw, "https://cdn.test/1.ts?token=SIGNED_FIXTURE").QueryValuesEqual || h.Get("Cookie") != "s=COOKIE_FIXTURE" {
		t.Fatal("traffic parsing")
	}
	if _, _, ok = trafficRequest("send: garbage", raw); ok {
		t.Fatal("malformed traffic accepted")
	}
}
