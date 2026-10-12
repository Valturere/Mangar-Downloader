// Package logging contains the common privacy boundary for technical output.
package logging

import (
	"regexp"
	"strings"
)

var urlPattern = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)
var credentialPattern = regexp.MustCompile(`(?im)(["']?(?:authorization|proxy-authorization|cookie|set-cookie|referer|origin|user-agent|accept|accept-language|x-[a-z0-9_-]*|token|sig|signature|password|passwd|secret|api[_-]?key)["']?\s*[=:]\s*).*`)

// Redact preserves tool language, but strips URL paths, queries and credentials.
func Redact(value string) string {
	value = urlPattern.ReplaceAllStringFunc(value, func(raw string) string {
		start := strings.Index(raw, "://") + 3
		end := len(raw)
		for i := start; i < len(raw); i++ {
			if strings.ContainsRune("/?#\\", rune(raw[i])) {
				end = i
				break
			}
		}
		host := raw[start:end]
		if at := strings.LastIndex(host, "@"); at >= 0 {
			host = host[at+1:]
		}
		return raw[:start] + host + "/[redacted]"
	})
	return credentialPattern.ReplaceAllString(value, "${1}[redacted]")
}
