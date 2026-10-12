package download

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Runtime capability probe uses the acquired managed binary. Never installs a
// dependency or cycles through guessed targets. Probe output contains no URLs.
func availableBrowserTarget(ctx context.Context, run Runner, exe, dir string, h *HTTPContext) string {
	client := "chrome"
	if h != nil && strings.Contains(h.UserAgent, "Edg/") {
		client = "edge"
	}
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	highest := 0
	target := ""
	_, err := run(probe, exe, []string{"--ignore-config", "--no-plugin-dirs", "--no-remote-components", "--list-impersonate-targets"}, dir, func(_, line string) {
		fields := strings.Fields(strings.ToLower(line))
		if len(fields) < 3 || fields[1] != "windows-10" || fields[2] != "curl_cffi" || !strings.HasPrefix(fields[0], client+"-") {
			return
		}
		v, e := strconv.Atoi(strings.TrimPrefix(fields[0], client+"-"))
		if e == nil && v > highest {
			highest = v
			target = fields[0] + ":" + fields[1]
		}
	})
	if err != nil {
		return ""
	}
	return target
}
