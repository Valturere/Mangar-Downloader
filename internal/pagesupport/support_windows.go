//go:build windows

// Package pagesupport matches URLs using the installed yt-dlp's own suitable()
// rules. It never extracts metadata, downloads media, or prints the URL.
package pagesupport

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/tools"
)

type matches struct {
	url, extractor   string
	found, supported bool
}

func (m *matches) line(line string) {
	if strings.HasPrefix(line, "  ") {
		if line == "  "+m.url && m.extractor != "" {
			m.found = true
			name := strings.ToLower(strings.TrimSuffix(m.extractor, " (CURRENTLY BROKEN)"))
			if name != "generic" && !strings.HasPrefix(name, "generic:") && name != "unsupportedurl" && name != "unsupported" && name != "commonmistakes" {
				m.supported = true
			}
		}
		return
	}
	m.extractor = strings.TrimSpace(line)
}

func Check(ctx context.Context, toolsRoot, rawURL string) (bool, error) {
	url, err := browserbridge.ValidatePageURL(rawURL)
	if err != nil {
		return false, errors.New("invalid URL")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	exe, err := tools.ReadOnlyYtDlp(ctx, toolsRoot)
	if err != nil {
		return false, errors.New("managed yt-dlp unavailable")
	}
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-remote-components", "--no-js-runtimes", "--encoding", "utf-8", "--list-extractors", "--", url}
	match := matches{url: url}
	bytes := 0
	code, err := process.RunStreaming(ctx, exe, args, filepath.Dir(exe), func(stream, line string) {
		bytes += len(line) + 1
		if bytes > 512<<10 {
			cancel()
			return
		}
		if stream == "stdout" {
			match.line(line)
		}
	})
	if err != nil || code != 0 || ctx.Err() != nil || !match.found {
		return false, errors.New("support query failed")
	}
	return match.supported, nil
}
