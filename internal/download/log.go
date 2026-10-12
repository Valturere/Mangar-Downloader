package download

import (
	"YTDLP-GUI-Mangar/internal/logging"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func redact(value string) string { return logging.Redact(value) }

type jobLog struct {
	mu             sync.Mutex
	file           *os.File
	size           int64
	id             string
	httpContext    *HTTPContext
	browserContext *BrowserContext
	browserCookies bool
	sourceURL      string
}

func newLog(dir, id string) (*jobLog, error) {
	dir = filepath.Join(dir, "jobs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, id+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &jobLog{file: f, id: id}, nil
}

// Cap each job log to 8 MiB. No raw credentials or unbounded in-memory capture.
func (l *jobLog) write(phase, process, stream, message string, code *uint32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil || l.size >= 8<<20 {
		return
	}
	message = redactSecrets(message, l.httpContext, l.browserContext)
	// Tools sometimes echo a query value without the surrounding URL/key.
	if u, err := url.Parse(l.sourceURL); err == nil {
		for _, values := range u.Query() {
			for _, value := range values {
				if len(value) < 4 {
					continue
				}
				for _, escaped := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
					message = strings.ReplaceAll(message, escaped, "[redacted]")
				}
			}
		}
	}
	if l.browserCookies {
		message = redactBrowserOutput(message)
	}
	b, _ := json.Marshal(map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "job_id": l.id, "phase": phase, "process": process, "stream": stream, "exit_code": code, "app_message": "Выполнение задания", "detail": redact(message)})
	n, err := l.file.Write(append(b, '\n'))
	l.size += int64(n)
	if err != nil {
		l.size = 8 << 20
	}
}

func redactSecrets(text string, h *HTTPContext, b *BrowserContext) string {
	values := []string{}
	for _, name := range h.headerNames() {
		values = append(values, h.headerValue(name))
	}
	if b != nil {
		for _, c := range b.Cookies {
			values = append(values, c.Value)
		}
	}
	for _, v := range values {
		if v == "" {
			continue
		}
		escaped, _ := json.Marshal(v)
		for _, value := range []string{v, url.QueryEscape(v), url.PathEscape(v), strings.Trim(string(escaped), "\"")} {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	return redact(text)
}
func (l *jobLog) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}
func (l *jobLog) exit(phase, exe string, code uint32, err error) {
	l.write(phase, filepath.Base(exe), "exit", fmt.Sprintf("Завершение процесса: %v", err), &code)
}
