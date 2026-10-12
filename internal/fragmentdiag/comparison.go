// Package fragmentdiag is developer tooling, independent of the production queue.
// Raw URL/header context lives only for one bounded native diagnostic process.
package fragmentdiag

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"YTDLP-GUI-Mangar/internal/download"
)

const TTL = 30 * time.Second
const MaxReplays = 2

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type Redirect struct {
	URL    string `json:"url"`
	Status int    `json:"status"`
	Target string `json:"target"`
}
type Manifest struct {
	URL  string               `json:"url"`
	HTTP download.HTTPContext `json:"http"`
}
type Response struct {
	Status      int      `json:"status"`
	FromCache   bool     `json:"fromCache"`
	HeaderNames []string `json:"headerNames"`
	ContentType string   `json:"contentType"`
}
type Capture struct {
	Version       int                  `json:"version"`
	Type          string               `json:"type"`
	CapturedAt    time.Time            `json:"capturedAt"`
	RequestID     string               `json:"requestId"`
	TabID         int                  `json:"tabId"`
	FrameID       int                  `json:"frameId"`
	Initiator     string               `json:"initiator"`
	Method        string               `json:"method"`
	URL           string               `json:"url"`
	Headers       []Header             `json:"headers"`
	Redirects     []Redirect           `json:"redirects"`
	Response      Response             `json:"response"`
	Manifests     []Manifest           `json:"manifests"`
	ManagedHTTP   download.HTTPContext `json:"managedHTTP"`
	ManagedCookie string               `json:"managedCookie"`
}

// Deliberately closed schema: there is no raw request, error text or header value.
type URLDiff struct {
	SchemeEqual       bool     `json:"scheme_equal"`
	HostEqual         bool     `json:"host_equal"`
	PortEqual         bool     `json:"port_equal"`
	PathEqual         bool     `json:"path_equal"`
	RawPathEqual      bool     `json:"raw_path_equal"`
	QueryNamesEqual   bool     `json:"query_names_equal"`
	QueryValuesEqual  bool     `json:"query_values_equal"`
	RawQueryEqual     bool     `json:"raw_query_equal"`
	QueryCountBrowser int      `json:"query_count_browser"`
	QueryCountReplay  int      `json:"query_count_replay"`
	DuplicatesBrowser bool     `json:"duplicates_browser"`
	DuplicatesReplay  bool     `json:"duplicates_replay"`
	MissingQueryNames []string `json:"missing_query_names"`
	BrowserQueryNames []string `json:"browser_query_names"`
	ReplayQueryNames  []string `json:"replay_query_names"`
}
type HeaderDiff struct {
	BrowserOnly           []string `json:"browser_only"`
	ReplayOnly            []string `json:"replay_only"`
	ValuesDiffer          []string `json:"values_differ"`
	RefererEqual          bool     `json:"referer_equal"`
	OriginEqual           bool     `json:"origin_equal"`
	UserAgentEqual        bool     `json:"user_agent_equal"`
	CookieEqual           bool     `json:"cookie_equal"`
	RangeEqual            bool     `json:"range_equal"`
	CookieBrowser         bool     `json:"cookie_browser"`
	CookieReplay          bool     `json:"cookie_replay"`
	AuthorizationObserved bool     `json:"authorization_observed"`
	RangeBrowser          bool     `json:"range_browser"`
	RangeReplay           bool     `json:"range_replay"`
}
type Replay struct {
	Status              int         `json:"status"`
	BodyStart           bool        `json:"body_start"`
	SentRequests        int         `json:"sent_requests"`
	RedirectStatuses    []int       `json:"redirect_statuses"`
	RedirectHosts       []string    `json:"redirect_hosts"`
	ResponseHeaderNames []string    `json:"response_header_names"`
	ContentType         string      `json:"content_type"`
	URL                 *URLDiff    `json:"url,omitempty"`
	Headers             *HeaderDiff `json:"headers,omitempty"`
	Result              string      `json:"result"`
}
type Report struct {
	BrowserStatus           int        `json:"browser_status"`
	BrowserRedirectStatuses []int      `json:"browser_redirect_statuses"`
	BrowserRedirectHosts    []string   `json:"browser_redirect_hosts"`
	HeaderVisibility        string     `json:"header_visibility"`
	ProductionHeaders       HeaderDiff `json:"production_headers"`
	Exact                   Replay     `json:"exact"`
	Ytdlp                   Replay     `json:"ytdlp"`
	ManifestMatched         bool       `json:"manifest_matched"`
	ManifestRequests        int        `json:"manifest_requests"`
	VariantQueryPresent     bool       `json:"variant_query_present"`
	SegmentURIAbsolute      bool       `json:"segment_uri_absolute"`
	SegmentURIQueryPresent  bool       `json:"segment_uri_query_present"`
	ResolvedURL             *URLDiff   `json:"resolved_url,omitempty"`
	FragmentReplays         int        `json:"fragment_replays"`
	Result                  string     `json:"result"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func names(v url.Values) []string {
	out := []string{}
	for k := range v {
		if safeName.MatchString(k) {
			out = append(out, k)
		} else {
			out = append(out, "[nonstandard]")
		}
	}
	slices.Sort(out)
	return out
}
func compareURL(a, b string) *URLDiff {
	x, ex := url.Parse(a)
	y, ey := url.Parse(b)
	if ex != nil || ey != nil {
		return nil
	}
	xq, ex := url.ParseQuery(x.RawQuery)
	yq, ey := url.ParseQuery(y.RawQuery)
	if ex != nil || ey != nil {
		return nil
	}
	xn, yn := names(xq), names(yq)
	d := &URLDiff{SchemeEqual: x.Scheme == y.Scheme, HostEqual: strings.EqualFold(x.Hostname(), y.Hostname()), PortEqual: x.Port() == y.Port(), PathEqual: x.Path == y.Path, RawPathEqual: x.EscapedPath() == y.EscapedPath(), QueryNamesEqual: reflect.DeepEqual(xn, yn), QueryValuesEqual: reflect.DeepEqual(xq, yq), RawQueryEqual: x.RawQuery == y.RawQuery, MissingQueryNames: []string{}}
	d.BrowserQueryNames, d.ReplayQueryNames = xn, yn
	// Equality uses real keys in RAM; safe display placeholders must not collapse
	// two different unusual parameter names into a false equality.
	d.QueryNamesEqual = len(xq) == len(yq)
	for name := range xq {
		if _, ok := yq[name]; !ok {
			d.QueryNamesEqual = false
		}
	}
	for _, v := range xq {
		d.QueryCountBrowser += len(v)
		d.DuplicatesBrowser = d.DuplicatesBrowser || len(v) > 1
	}
	for _, v := range yq {
		d.QueryCountReplay += len(v)
		d.DuplicatesReplay = d.DuplicatesReplay || len(v) > 1
	}
	missing := url.Values{}
	for n := range xq {
		if _, ok := yq[n]; !ok {
			missing[n] = nil
		}
	}
	d.MissingQueryNames = names(missing)
	return d
}
func headerNames(h http.Header) []string {
	r := []string{}
	for n := range h {
		r = append(r, strings.ToLower(n))
	}
	slices.Sort(r)
	return r
}
func compareHeaders(a, b http.Header) *HeaderDiff {
	d := &HeaderDiff{BrowserOnly: []string{}, ReplayOnly: []string{}, ValuesDiffer: []string{}, RefererEqual: a.Get("Referer") == b.Get("Referer"), OriginEqual: a.Get("Origin") == b.Get("Origin"), UserAgentEqual: a.Get("User-Agent") == b.Get("User-Agent"), CookieEqual: a.Get("Cookie") == b.Get("Cookie"), RangeEqual: a.Get("Range") == b.Get("Range"), CookieBrowser: a.Get("Cookie") != "", CookieReplay: b.Get("Cookie") != "", AuthorizationObserved: a.Get("Authorization") != "", RangeBrowser: a.Get("Range") != "", RangeReplay: b.Get("Range") != ""}
	for _, n := range headerNames(a) {
		if _, ok := b[http.CanonicalHeaderKey(n)]; !ok {
			d.BrowserOnly = append(d.BrowserOnly, n)
		} else if !slices.Equal(a.Values(n), b.Values(n)) {
			d.ValuesDiffer = append(d.ValuesDiffer, n)
		}
	}
	for _, n := range headerNames(b) {
		if _, ok := a[http.CanonicalHeaderKey(n)]; !ok {
			d.ReplayOnly = append(d.ReplayOnly, n)
		}
	}
	return d
}
func allowed(n string) bool {
	n = strings.ToLower(n)
	return slices.Contains([]string{"referer", "origin", "user-agent", "accept", "accept-language", "accept-encoding", "cookie", "range", "cache-control", "pragma", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest", "sec-fetch-user", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"}, n) || (strings.HasPrefix(n, "x-") && !strings.Contains(n, "auth") && !strings.Contains(n, "token") && !strings.Contains(n, "secret") && !strings.Contains(n, "key"))
}
func validURL(raw string) bool {
	if len(raw) > 16384 {
		return false
	}
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	_, e = url.ParseQuery(u.RawQuery)
	return e == nil
}
func Validate(c *Capture, now time.Time) (http.Header, error) {
	bad := errors.New("Данные диагностики отклонены")
	if c == nil || c.Version != 1 || c.Type != "fragment_diagnostic" || c.Method != "GET" || c.TabID < 0 || c.FrameID < 0 || len(c.RequestID) > 128 || c.RequestID == "" || !validURL(c.URL) || c.CapturedAt.IsZero() || now.Sub(c.CapturedAt) > TTL || c.CapturedAt.After(now.Add(time.Second)) || !slices.Contains([]int{200, 206}, c.Response.Status) || c.Response.FromCache || len(c.Headers) > 64 || len(c.Redirects) > 5 || len(c.Manifests) > 2 {
		return nil, bad
	}
	h := http.Header{}
	for _, v := range c.Headers {
		n := http.CanonicalHeaderKey(v.Name)
		if !safeName.MatchString(n) || len(v.Value) > 8192 || strings.ContainsAny(v.Value, "\r\n\x00") {
			return nil, bad
		}
		if _, ok := h[n]; ok {
			return nil, bad
		}
		h.Set(n, v.Value)
	}
	for _, m := range c.Manifests {
		if !validURL(m.URL) {
			return nil, bad
		}
		m.HTTP.Captured = true
		if _, e := download.ValidateCapturedHTTP(&m.HTTP); e != nil {
			return nil, bad
		}
	}
	for _, r := range c.Redirects {
		if !validURL(r.URL) || !validURL(r.Target) || r.Status < 300 || r.Status > 399 {
			return nil, bad
		}
	}
	c.ManagedHTTP.Captured = true
	if _, e := download.ValidateCapturedHTTP(&c.ManagedHTTP); e != nil || len(c.ManagedCookie) > 8192 || strings.ContainsAny(c.ManagedCookie, "\r\n\x00") {
		return nil, bad
	}
	return h, nil
}

func productionHeaders(c *Capture) http.Header {
	out := http.Header{}
	for n, v := range c.ManagedHTTP.Headers {
		out.Set(n, v)
	}
	for n, v := range map[string]string{"Referer": c.ManagedHTTP.Referer, "Origin": c.ManagedHTTP.Origin, "User-Agent": c.ManagedHTTP.UserAgent, "Cookie": c.ManagedCookie} {
		if v != "" {
			out.Set(n, v)
		}
	}
	return out
}
