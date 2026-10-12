package download

import (
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
)

const metaPrefix = "MANGAR_META:"
const progressPrefix = "MANGAR_PROGRESS:"
const pathPrefix = "MANGAR_PATH:"
const postPrefix = "MANGAR_POST:"

// No network metadata preflight. JSON escaping keeps titles/newlines in one line.
func ytdlpArgs(paths map[string]string, temp, source string, httpContext ...*HTTPContext) []string {
	args := []string{"--ignore-config", "--no-plugin-dirs", "--no-remote-components", "--no-js-runtimes", "--js-runtimes", "deno:" + paths["deno.exe"],
		"--ffmpeg-location", filepath.Dir(paths["ffmpeg.exe"]), "--no-playlist", "--playlist-items", "1", "--no-simulate", "--encoding", "utf-8", "--newline", "--progress", "--progress-delta", "0.2", "-N", "8",
		"-f", "bv+ba/b", "--merge-output-format", "mkv", "-o", filepath.Join(temp, "media.%(ext)s"),
		"--print", "before_dl:" + metaPrefix + "%(.{id,title,format_id,vcodec,acodec,width,height,fps})j",
		"--print", "after_move:" + pathPrefix + "%(filepath)j",
		"--progress-template", "download:" + progressPrefix + "%(progress.{downloaded_bytes,total_bytes,total_bytes_estimate,speed,eta,status})j",
		"--progress-template", "postprocess:" + postPrefix + "%(progress.status)j"}
	if len(httpContext) > 0 {
		args = append(args, httpContext[0].args()...)
	}
	return append(args, "--", source)
}

type metadata struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}
type progress struct {
	Downloaded *float64 `json:"downloaded_bytes"`
	Total      *float64 `json:"total_bytes"`
	Estimate   *float64 `json:"total_bytes_estimate"`
	Speed      *float64 `json:"speed"`
	ETA        *float64 `json:"eta"`
	FormatID   string   `json:"format_id"`
}

func parseProgress(line string) (progress, *float64, bool) {
	var p progress
	if !strings.HasPrefix(line, progressPrefix) || json.Unmarshal([]byte(strings.TrimPrefix(line, progressPrefix)), &p) != nil {
		return p, nil, false
	}
	total := p.Total
	if total == nil {
		total = p.Estimate
	}
	var percent *float64
	if p.Downloaded != nil && total != nil && *total > 0 && *p.Downloaded >= 0 {
		value := math.Min(100, *p.Downloaded / *total * 100)
		percent = &value
	}
	if p.Speed != nil && *p.Speed <= 0 {
		p.Speed = nil
	}
	if p.ETA != nil && *p.ETA < 0 {
		p.ETA = nil
	}
	return p, percent, true
}
