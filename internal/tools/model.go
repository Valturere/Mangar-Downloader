// Package tools управляет только собственными зависимостями, без Wails и PATH.
package tools

import (
	"context"
	"fmt"
	"time"
)

type Tool string

const (
	YtDlp  Tool = "yt-dlp"
	FFmpeg Tool = "ffmpeg"
	Deno   Tool = "deno"
)

var toolOrder = []Tool{YtDlp, FFmpeg, Deno}

func executables(t Tool) []string {
	switch t {
	case YtDlp:
		return []string{"yt-dlp.exe"}
	case FFmpeg:
		return []string{"ffmpeg.exe", "ffprobe.exe"}
	case Deno:
		return []string{"deno.exe"}
	}
	return nil
}

type Status struct {
	AuxiliaryRepair bool   `json:"auxiliaryRepair"`
	Tool            Tool   `json:"tool"`
	State           string `json:"state"`
	Version         string `json:"version"`
	Message         string `json:"message"`
	Bytes           int64  `json:"bytes"`
	Total           int64  `json:"total"`
}
type Snapshot struct {
	Progress    *PreparationProgress `json:"progress"`
	Preparation string               `json:"preparation"`
	Busy        bool                 `json:"busy"`
	Revision    uint64               `json:"revision"`
	Tools       []Status             `json:"tools"`
	Error       string               `json:"error"`
}
type Asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
	URL    string `json:"browser_download_url"`
}
type Release struct {
	ID         int64   `json:"id"`
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []Asset `json:"assets"`
}
type Candidate struct {
	Tool       Tool
	Repository string
	ReleaseID  int64
	Tag        string
	Asset      Asset
	Checksum   *Asset
}

func (c Candidate) Identity() string { return fmt.Sprintf("r%d-a%d", c.ReleaseID, c.Asset.ID) }

type Source interface {
	Latest(context.Context, Tool) (Candidate, error)
}
type Verifier interface {
	Verify(context.Context, Tool, string) (map[string]string, error)
}
type Manifest struct {
	Schema      int               `json:"schema_version"`
	Tool        Tool              `json:"tool"`
	Identity    string            `json:"identity"`
	Repository  string            `json:"repository"`
	ReleaseID   int64             `json:"release_id"`
	Tag         string            `json:"release_tag"`
	Asset       Asset             `json:"asset"`
	InstalledAt time.Time         `json:"installed_at"`
	Executables map[string]string `json:"executables"`
	Versions    map[string]string `json:"versions"`
	SHA256      string            `json:"sha256"`
	Integrity   string            `json:"integrity"`
	Files       map[string]string `json:"file_sha256"`
}
type ToolError struct {
	Message string
	Detail  string
	Code    string
}

func (e *ToolError) Error() string { return e.Message }
func failure(code, message, detail string) error {
	return &ToolError{Code: code, Message: message, Detail: detail}
}
