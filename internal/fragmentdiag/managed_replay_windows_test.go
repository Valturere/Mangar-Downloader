package fragmentdiag

import (
	"YTDLP-GUI-Mangar/internal/process"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var replyStatus = regexp.MustCompile(`^reply: .*HTTP/[0-9.]+ ([0-9]{3})`)

// Python http.client bytes repr parser. Never return raw parse errors.
func decodeTraffic(s string) (string, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "send:"))
	if len(s) < 3 || s[0] != 'b' || (s[1] != '\'' && s[1] != '"') || s[len(s)-1] != s[1] {
		return "", false
	}
	quote := s[1]
	s = s[2 : len(s)-1]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			return "", false
		}
		switch s[i] {
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case '\'', '"':
			if s[i] != quote {
				return "", false
			}
			b.WriteByte(s[i])
		case 'x':
			if i+2 >= len(s) {
				return "", false
			}
			v, e := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if e != nil {
				return "", false
			}
			b.WriteByte(byte(v))
			i += 2
		default:
			return "", false
		}
	}
	return b.String(), true
}
func trafficRequest(line, original string) (string, http.Header, bool) {
	s, ok := decodeTraffic(line)
	if !ok {
		return "", nil, false
	}
	parts := strings.Split(s, "\r\n")
	first := strings.Fields(parts[0])
	if len(first) != 3 || first[0] != "GET" {
		return "", nil, false
	}
	h := http.Header{}
	for _, p := range parts[1:] {
		if p == "" {
			break
		}
		n, v, ok := strings.Cut(p, ":")
		if !ok {
			return "", nil, false
		}
		h.Add(n, strings.TrimSpace(v))
	}
	u, e := url.Parse(original)
	if e != nil {
		return "", nil, false
	}
	target, e := url.Parse(first[1])
	if e != nil {
		return "", nil, false
	}
	if !target.IsAbs() {
		target.Scheme = u.Scheme
		target.Host = h.Get("Host")
	}
	h.Del("Host")
	h.Del("Connection") // Managed transport supplies these itself.
	return target.String(), h, true
}

func ytdlpReplay(ctx context.Context, exe, raw string, h http.Header) Replay {
	r := Replay{Result: "request_failed"}
	info := map[string]any{"id": "fragment-diagnostic", "title": "fragment-diagnostic", "url": raw, "ext": "ts", "protocol": "https", "http_headers": map[string]string{}}
	u, _ := url.Parse(raw)
	info["protocol"] = u.Scheme
	for n := range h {
		info["http_headers"].(map[string]string)[n] = h.Get(n)
	}
	b, _ := json.Marshal(info)
	defer clear(b)
	child, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-remote-components", "--proxy", "", "--load-info-json", "-", "--retries", "0", "--fragment-retries", "0", "--extractor-retries", "0", "--file-access-retries", "0", "--no-continue", "--no-part", "--no-cache-dir", "--fixup", "never", "--no-progress", "--no-colors", "--print-traffic", "--socket-timeout", "5", "-o", os.DevNull}
	_, e := process.RunStreamingWithInput(child, exe, args, "", b, func(_, line string) {
		if strings.HasPrefix(line, "send:") {
			r.SentRequests++
			if rawRequest, headers, ok := trafficRequest(line, raw); ok {
				r.URL = compareURL(raw, rawRequest)
				r.Headers = compareHeaders(h, headers)
			}
			if r.SentRequests > 1 {
				cancel()
			}
		}
		if m := replyStatus.FindStringSubmatch(line); len(m) == 2 {
			r.Status, _ = strconv.Atoi(m[1])
			// Stop on first response headers: no body/redirect/resume retry.
			if r.Status >= 300 && r.Status < 400 {
				r.RedirectStatuses = append(r.RedirectStatuses, r.Status)
			}
			cancel()
		}
		// Raw stdout/stderr, paths, headers, errors are discarded here in RAM.
	})
	if r.Status != 0 {
		r.Result = "headers_received_body_not_checked"
	} else if e == nil {
		r.Result = "status_not_observed"
	}
	return r
}
