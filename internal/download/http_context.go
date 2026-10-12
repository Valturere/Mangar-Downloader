package download

import (
	"errors"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
)

// HTTPContext never enters snapshots. Captured is set only by the native adapter.
type HTTPContext struct {
	Referer   string            `json:"referer,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	UserAgent string            `json:"userAgent,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Captured  bool              `json:"-"`
}

// Additional headers use an allowlist: transport/authentication require a
// separate explicit design rather than bypassing this security boundary.
var extraHTTPHeaders = map[string]bool{"Accept": true, "Accept-Language": true}

func normalizeHTTPContext(input *HTTPContext) (*HTTPContext, error) {
	if input == nil {
		return nil, nil
	}
	bad := errors.New("Недопустимый HTTP context: проверьте заголовки запроса")
	out := &HTTPContext{Referer: input.Referer, Origin: input.Origin, UserAgent: input.UserAgent, Captured: input.Captured, Headers: map[string]string{}}
	if len(input.Headers) > 32 {
		return nil, bad
	}
	validValue := func(value string) bool {
		if len(value) > 8192 {
			return false
		}
		for _, ch := range value {
			if ch < 32 || ch == 127 {
				return false
			}
		}
		return true
	}
	for _, value := range []string{out.Referer, out.Origin, out.UserAgent} {
		if !validValue(value) {
			return nil, bad
		}
	}
	for i, value := range []string{out.Referer, out.Origin} {
		if value == "" || (input.Captured && i == 1 && value == "null") {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Fragment != "" {
			return nil, bad
		}
	}
	if out.Origin != "" && out.Origin != "null" {
		u, _ := url.Parse(out.Origin)
		if u.Path != "" || u.RawQuery != "" || u.ForceQuery {
			return nil, bad
		}
	}
	for name, value := range input.Headers {
		if name == "" || len(name) > 128 || !validValue(value) {
			return nil, bad
		}
		for _, ch := range name {
			if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", ch)) {
				return nil, bad
			}
		}
		name = textproto.CanonicalMIMEHeaderKey(name)
		if !extraHTTPHeaders[name] && !(input.Captured && browserHeaderAllowed(name)) {
			return nil, bad
		}
		if _, exists := out.Headers[name]; exists {
			return nil, bad
		}
		out.Headers[name] = value
	}
	return out, nil
}

func (c *HTTPContext) headerNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.Headers)+3)
	if c.Referer != "" {
		names = append(names, "Referer")
	}
	if c.Origin != "" {
		names = append(names, "Origin")
	}
	if c.UserAgent != "" {
		names = append(names, "User-Agent")
	}
	for name := range c.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (c *HTTPContext) headerValue(name string) string {
	switch name {
	case "Referer":
		return c.Referer
	case "Origin":
		return c.Origin
	case "User-Agent":
		return c.UserAgent
	default:
		return c.Headers[name]
	}
}
func (c *HTTPContext) args() []string {
	var args []string
	for _, name := range c.headerNames() {
		args = append(args, "--add-headers", name+":"+c.headerValue(name))
	}
	return args
}

type httpProfile struct {
	id, host   string
	pathSuffix string
	context    HTTPContext
}

// Registry contains only experimentally proven profiles. An empty registry is
// intentional until a real manifest AND media request proves the minimum set.
var httpProfiles = []httpProfile{}

func matchesHTTPHost(host, domain string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	return host == domain || strings.HasSuffix(host, "."+domain)
}
func resolveHTTPProfile(raw string, profiles []httpProfile) (string, *HTTPContext) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", nil
	}
	for _, p := range profiles {
		if matchesHTTPHost(u.Hostname(), p.host) && (p.pathSuffix == "" || strings.HasSuffix(strings.ToLower(u.Path), p.pathSuffix)) {
			c := p.context
			return p.id, &c
		}
	}
	return "", nil
}
func prepareHTTPRequest(r Request) (Request, error) {
	r.httpProfile = "" // External context never supplies a trusted provider identity.
	if r.HTTPContext == nil {
		r.httpProfile, r.HTTPContext = resolveHTTPProfile(r.URL, httpProfiles)
	}
	c, err := normalizeHTTPContext(r.HTTPContext)
	r.HTTPContext = c
	if err == nil && r.Browser != nil {
		r.Browser, err = normalizeBrowserContext(r.URL, r.Browser)
	}
	return r, err
}

// Broader than provider profiles, but still explicitly excludes credentials,
// hop-by-hop, transport, cache/range and content negotiation unsupported by replay.
func browserHeaderAllowed(name string) bool {
	n := strings.ToLower(name)
	if n == "x-api-key" || strings.Contains(n, "token") || strings.Contains(n, "auth") || strings.Contains(n, "secret") || strings.Contains(n, "key") {
		return false
	}
	return strings.HasPrefix(n, "x-") || n == "sec-fetch-site" || n == "sec-fetch-mode" || n == "sec-fetch-dest" || n == "sec-fetch-user" || n == "sec-ch-ua" || n == "sec-ch-ua-mobile" || n == "sec-ch-ua-platform"
}

// ValidateCapturedHTTP is shared by the IPC boundary and scheduler admission.
func ValidateCapturedHTTP(c *HTTPContext) (*HTTPContext, error) { return normalizeHTTPContext(c) }
