package tools

import (
	"YTDLP-GUI-Mangar/internal/logging"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ToolLogger bounds diagnostics to three 2 MiB files. It never receives request URLs.
func ToolLogger(dir string) func(Tool, string, string) {
	var mu sync.Mutex
	return func(t Tool, phase, detail string) {
		mu.Lock()
		defer mu.Unlock()
		if os.MkdirAll(dir, 0700) != nil {
			return
		}
		p := filepath.Join(dir, "tools.jsonl")
		// Defensive redaction also covers URLs printed by failed external binaries.
		detail = logging.Redact(detail)
		if len(detail) > 256<<10 {
			detail = detail[:256<<10] + " [truncated]"
		}
		b, _ := json.Marshal(map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339Nano), "tool": t, "phase": phase, "app_message": "Операция с компонентом", "detail": detail})
		if s, e := os.Stat(p); e == nil && s.Size()+int64(len(b))+1 > 2<<20 {
			os.Remove(p + ".2")
			os.Rename(p+".1", p+".2")
			if os.Rename(p, p+".1") != nil {
				return
			}
		}
		f, e := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return
		}
		defer f.Close()
		f.Write(append(b, '\n'))
	}
}
