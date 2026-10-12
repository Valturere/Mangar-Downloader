// Package download owns the in-memory FIFO scheduler, independently of Wails.
package download

import (
	"YTDLP-GUI-Mangar/internal/tools"
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

type Request struct {
	URL         string          `json:"url"`
	Directory   string          `json:"directory"`
	HTTPContext *HTTPContext    `json:"httpContext,omitempty"`
	Browser     *BrowserContext `json:"-"`
	httpProfile string
	cookieMode  string // Backend snapshot at admission; retained for logical Retry.
}
type Snapshot struct {
	Revision uint64 `json:"revision"`
	Jobs     []Job  `json:"jobs"`
	Active   int    `json:"active"`
	Closing  bool   `json:"closing"`
}
type Job struct {
	BatchID        string     `json:"batchID"`
	BatchIndex     int        `json:"batchIndex"`
	BatchSource    string     `json:"batchSource"`
	OutputFilename string     `json:"outputFilename"`
	InvalidSource  bool       `json:"invalidSource"`
	ID             string     `json:"id"`
	ExecutionID    string     `json:"executionID"`
	Sequence       uint64     `json:"sequence"`
	Attempt        int        `json:"attempt"`
	Source         string     `json:"source"`
	CreatedAt      time.Time  `json:"createdAt"`
	StartedAt      *time.Time `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt"`
	Error          string     `json:"error"`
	State          string     `json:"state"`
	Active         bool       `json:"active"`
	Title          string     `json:"title"`
	Message        string     `json:"message"`
	Percent        *float64   `json:"percent"`
	Speed          *float64   `json:"speed"`
	ETA            *float64   `json:"eta"`
	SpeedAt        *time.Time `json:"speedAt"`
	ETAAt          *time.Time `json:"etaAt"`
	FormatID       string     `json:"formatID"`
	FinalPath      string     `json:"finalPath"`
}
type ToolProvider interface {
	Acquire(context.Context, tools.Tool) (*tools.Lease, error)
}
type Runner func(context.Context, string, []string, string, func(string, string)) (uint32, error)

func ValidateURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u == nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.ContainsAny(raw, "\x00\r\n") {
		return "", errors.New("Некорректная ссылка. Используйте http:// или https://")
	}
	return raw, nil
}
