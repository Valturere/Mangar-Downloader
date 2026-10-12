package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type GitHubSource struct {
	HTTP *HTTP
	API  string
}

func repository(t Tool) string {
	switch t {
	case YtDlp:
		return "yt-dlp/yt-dlp"
	case FFmpeg:
		return "yt-dlp/FFmpeg-Builds"
	case Deno:
		return "denoland/deno"
	}
	return ""
}
func (s GitHubSource) Latest(ctx context.Context, t Tool) (Candidate, error) {
	repo := repository(t)
	if repo == "" {
		return Candidate{}, failure("tool", "Неизвестный компонент", "unknown tool")
	}
	api := s.API
	if api == "" {
		api = "https://api.github.com"
	}
	b, err := s.HTTP.Small(ctx, api+"/repos/"+repo+"/releases/latest", 4<<20)
	if err != nil {
		return Candidate{}, err
	}
	var r Release
	if err = json.Unmarshal(b, &r); err != nil {
		return Candidate{}, failure("metadata", "Сервер вернул некорректные данные выпуска", "invalid JSON")
	}
	if r.ID <= 0 || r.Draft || r.Prerelease || !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(r.Tag) {
		return Candidate{}, failure("metadata", "Не найден стабильный выпуск компонента", "release identity")
	}
	var selected *Asset
	for i := range r.Assets {
		a := &r.Assets[i]
		match := false
		switch t {
		case YtDlp:
			match = a.Name == "yt-dlp.exe"
		case Deno:
			match = a.Name == "deno-x86_64-pc-windows-msvc.zip"
		case FFmpeg:
			match = regexp.MustCompile(`^ffmpeg-.*-win64-gpl\.zip$`).MatchString(a.Name)
		}
		if match {
			if selected != nil {
				return Candidate{}, failure("metadata", "Выпуск содержит неоднозначные компоненты", "duplicate matching assets")
			}
			selected = a
		}
	}
	if selected == nil {
		return Candidate{}, failure("metadata", "В выпуске отсутствует Windows x64 компонент", "asset not found")
	}
	a := *selected
	// Bind the download to this repository, release tag and asset. UI never supplies URLs.
	prefix := "https://github.com/" + repo + "/releases/download/" + r.Tag + "/"
	if a.ID <= 0 || a.Size <= 0 || !strings.HasPrefix(a.URL, prefix) || strings.TrimPrefix(a.URL, prefix) != a.Name {
		return Candidate{}, failure("metadata", "Некорректные данные компонента", "asset identity/URL")
	}
	if a.Digest != "" && !regexp.MustCompile(`^sha256:[a-fA-F0-9]{64}$`).MatchString(a.Digest) {
		return Candidate{}, failure("integrity", "Неподдерживаемые данные целостности", "invalid digest")
	}
	c := Candidate{Tool: t, Repository: repo, ReleaseID: r.ID, Tag: r.Tag, Asset: a}
	checksumName := ""
	switch t {
	case YtDlp:
		checksumName = "SHA2-256SUMS"
	case FFmpeg:
		checksumName = "checksums.sha256"
	case Deno:
		checksumName = a.Name + ".sha256sum"
	}
	for _, check := range r.Assets {
		if check.Name == checksumName {
			if c.Checksum != nil {
				return Candidate{}, fmt.Errorf("Повторяющийся checksum asset")
			}
			if check.URL != prefix+checksumName || check.Size <= 0 || check.Size > 1<<20 {
				return Candidate{}, fmt.Errorf("Некорректный checksum asset")
			}
			copy := check
			c.Checksum = &copy
		}
	}
	return c, nil
}
func checksumFor(b []byte, name string) (string, error) {
	// Deno Windows CI publishes Format-List output from Get-FileHash.
	ps := regexp.MustCompile(`(?i)^\s*Algorithm\s*:\s*SHA256\s*\r?\nHash\s*:\s*([a-f0-9]{64})\s*\r?\nPath\s*:\s*([^\r\n]+)\s*$`).FindStringSubmatch(string(b))
	if len(ps) == 3 {
		p := strings.ReplaceAll(strings.TrimSpace(ps[2]), "\\", "/")
		if p[strings.LastIndex(p, "/")+1:] == name {
			return strings.ToLower(ps[1]), nil
		}
	}
	found := ""
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		if found != "" || !regexp.MustCompile(`^[a-fA-F0-9]{64}$`).MatchString(fields[0]) {
			return "", failure("integrity", "Некорректная контрольная сумма компонента", "checksum format/duplicate")
		}
		found = strings.ToLower(fields[0])
	}
	if found == "" {
		return "", failure("integrity", "В выпуске нет контрольной суммы компонента", "checksum absent")
	}
	return found, nil
}
