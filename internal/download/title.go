package download

import (
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var manifestTimestamp = regexp.MustCompile(`^(.+?)\s+\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}(?::\d{2})?$`)
var secretName = regexp.MustCompile(`(?i)(?:token|sig(?:nature)?|password|passwd|authorization|cookie|secret|api[_-]?key|credential)\s*[:=]|(?:^|[?&#])[^\s?&#=]{1,80}=`)

func sensitiveName(value string) bool {
	// Decode only for detection; never turn escaped URL material into a filename.
	for i := 0; i < 3; i++ {
		if strings.Contains(value, "://") || strings.ContainsAny(value, "?=") || secretName.MatchString(value) {
			return true
		}
		next, err := url.PathUnescape(value)
		if err != nil || next == value {
			break
		}
		value = next
	}
	return false
}

func titleText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(value)
}
func cleanTitle(value string) string {
	value = titleText(value)
	return string([]rune(value)[:min(len([]rune(value)), 512)])
}

// Metadata may echo a known query value as a bare title, with no token key.
func sourceSecretTitle(title, raw string) bool {
	title = titleText(title)
	variants := []string{title}
	for i := 0; i < 2; i++ {
		next, err := url.PathUnescape(title)
		if err != nil || next == title {
			break
		}
		title = titleText(next)
		variants = append(variants, title)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	for _, values := range u.Query() {
		for _, value := range values {
			if value == "" {
				continue
			}
			for i := 0; i < 3; i++ {
				for _, candidate := range variants {
					if (len(value) >= 4 && strings.Contains(strings.ToLower(candidate), strings.ToLower(value))) || strings.EqualFold(strings.TrimSpace(candidate), value) {
						return true
					}
				}
				next, e := url.PathUnescape(value)
				if e != nil || next == value {
					break
				}
				value = next
			}
		}
	}
	return false
}

func safeSource(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "HTTP(S)"
	}
	return u.Scheme + "://" + u.Host
}

// A small conservative classifier: short real titles (1917, Se7en) survive.
func genericTitle(title, source string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if match := manifestTimestamp.FindStringSubmatch(t); match != nil {
		return genericTitle(match[1], source)
	}
	if t == "" || strings.Contains(t, "hls:manifest") || strings.HasSuffix(t, ".m3u8") || strings.HasSuffix(t, ".mpd") || strings.Contains(t, "://") {
		return true
	}
	base := strings.TrimSuffix(t, path.Ext(t))
	switch base {
	case "master", "playlist", "index", "manifest", "video", "240", "360", "480", "720", "1080", "1440", "2160", "4320", "240p", "360p", "480p", "720p", "1080p", "1440p", "2160p", "4320p":
		return true
	}
	u, err := url.Parse(source)
	if err == nil {
		name := strings.ToLower(path.Base(u.Path))
		ext := path.Ext(name)
		media := ext == ".mp4" || ext == ".mkv" || ext == ".webm" || ext == ".m3u8" || ext == ".mpd" || ext == ".ts" || ext == ".m4v"
		if media && (t == name || t == strings.TrimSuffix(name, ext)) {
			return true
		}
	}
	return false
}
func displayTitle(title, source string) (string, bool) {
	if sensitiveName(title) {
		return "Видео", true
	}
	title = cleanTitle(title)
	if !genericTitle(title, source) {
		return redact(strings.TrimSpace(title)), false
	}
	u, err := url.Parse(source)
	if err != nil || u.Hostname() == "" {
		return "Видео", true
	}
	return "Видео — " + u.Hostname(), true
}
