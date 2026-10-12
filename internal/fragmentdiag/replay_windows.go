package fragmentdiag

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// No retrying transport, proxy environment, CookieJar, response body persistence
// or automatic redirect. Even a redirect cannot expand the request budget.
func exactReplay(ctx context.Context, raw string, h http.Header) Replay {
	r := Replay{Result: "request_failed", SentRequests: 1}
	// HTTP/2 can retry REFUSED_STREAM internally; disable it to keep one send.
	t := &http.Transport{DisableCompression: true, DisableKeepAlives: true, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
	defer t.CloseIdleConnections()
	client := &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(ctx, "GET", raw, nil)
	if e != nil {
		r.SentRequests = 0
		return r
	}
	req.Header = h.Clone()
	// net/http adds its own UA unless explicitly suppressed when absent.
	if _, ok := req.Header["User-Agent"]; !ok {
		req.Header["User-Agent"] = []string{""}
	}
	r.URL = compareURL(raw, req.URL.String())
	// Empty UA is a net/http suppression marker, not a sent header.
	generated := req.Header.Clone()
	if _, ok := h["User-Agent"]; !ok {
		generated.Del("User-Agent")
	}
	r.Headers = compareHeaders(h, generated)
	resp, e := client.Do(req)
	if e != nil {
		return r
	}
	defer resp.Body.Close()
	r.Status = resp.StatusCode
	r.ResponseHeaderNames = headerNames(resp.Header)
	r.ContentType = safeContentType(resp.Header.Get("Content-Type"))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		r.RedirectStatuses = []int{resp.StatusCode}
		if target, e := resp.Location(); e == nil {
			r.RedirectHosts = []string{target.Hostname()}
		}
		r.Result = "redirect_not_followed"
		return r
	}
	var b [1]byte
	n, _ := resp.Body.Read(b[:])
	r.BodyStart = n > 0
	r.Result = "response_received"
	return r
}
func safeContentType(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.Split(s, ";")[0]))
	switch s {
	case "video/mp2t", "video/mp4", "application/octet-stream", "text/html", "application/vnd.apple.mpegurl", "application/x-mpegurl":
		return s
	}
	return "other"
}

func inspectManifests(ctx context.Context, c *Capture, report *Report) {
	for _, m := range c.Manifests {
		if ctx.Err() != nil {
			return
		}
		h := http.Header{}
		for n, v := range m.HTTP.Headers {
			h.Set(n, v)
		}
		for n, v := range map[string]string{"Referer": m.HTTP.Referer, "Origin": m.HTTP.Origin, "User-Agent": m.HTTP.UserAgent} {
			if v != "" {
				h.Set(n, v)
			}
		}
		// Manifest analysis is separate from the fragment replay budget. No redirects.
		t := &http.Transport{DisableCompression: true, DisableKeepAlives: true, TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{}}
		client := http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, e := http.NewRequestWithContext(ctx, "GET", m.URL, nil)
		if e != nil {
			t.CloseIdleConnections()
			continue
		}
		req.Header = h
		report.ManifestRequests++
		resp, e := client.Do(req)
		if e != nil {
			t.CloseIdleConnections()
			continue
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 512*1024+1))
		resp.Body.Close()
		t.CloseIdleConnections()
		if readErr != nil || resp.StatusCode != 200 || len(b) > 512*1024 {
			clear(b)
			continue
		}
		base, _ := url.Parse(m.URL)
		var match *URLDiff
		var matchedURI *url.URL
		matches := 0
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			rel, e := url.Parse(line)
			if e != nil {
				continue
			}
			resolved := base.ResolveReference(rel).String()
			diff := compareURL(c.URL, resolved)
			if diff == nil || !diff.HostEqual || !diff.PathEqual {
				continue
			}
			// Only a media playlist establishes membership, never a guessed master URI.
			if !strings.Contains(string(b), "#EXTINF:") {
				continue
			}
			matches++
			match, matchedURI = diff, rel
			if diff.QueryValuesEqual && diff.RawQueryEqual && diff.RawPathEqual {
				matches = 1
				break
			}
		}
		clear(b)
		if matches == 1 {
			report.ManifestMatched = true
			report.ResolvedURL = match
			report.VariantQueryPresent = base.RawQuery != ""
			report.SegmentURIAbsolute = matchedURI.IsAbs()
			report.SegmentURIQueryPresent = matchedURI.RawQuery != ""
			return
		}
	}
}

func Run(ctx context.Context, c *Capture) Report {
	r := Report{Result: "invalid", HeaderVisibility: "webRequest_observable_only_not_wire_parity"}
	if c != nil {
		defer func() { *c = Capture{} }()
	}
	h, e := Validate(c, time.Now())
	if e != nil {
		return r
	}
	defer clear(h)
	r.BrowserStatus = c.Response.Status
	r.ProductionHeaders = *compareHeaders(h, productionHeaders(c))
	for _, v := range c.Redirects {
		u, _ := url.Parse(v.Target)
		r.BrowserRedirectStatuses = append(r.BrowserRedirectStatuses, v.Status)
		r.BrowserRedirectHosts = append(r.BrowserRedirectHosts, u.Hostname())
	}
	for n := range h {
		if !allowed(n) {
			r.Result = "blocked_header_boundary"
			return r
		}
	}
	if ctx.Err() != nil {
		r.Result = "cancelled"
		return r
	}
	deadline := c.CapturedAt.Add(TTL)
	cycle, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	r.Exact = exactReplay(cycle, c.URL, h)
	r.FragmentReplays += r.Exact.SentRequests
	if cycle.Err() != nil {
		r.Result = "cancelled_or_expired"
		return r
	}
	if len(r.Exact.RedirectStatuses) != 0 {
		r.Result = "redirect_boundary_no_further_replay"
		return r
	}
	r.Ytdlp.Result = "not_run_strict_public_request_budget"
	inspectManifests(cycle, c, &r)
	r.Result = "comparison_only_no_transport_claim"
	return r
}
