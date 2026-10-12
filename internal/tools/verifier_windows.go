//go:build windows

package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"YTDLP-GUI-Mangar/internal/process"
)

type ProcessVerifier struct{}

func (v ProcessVerifier) Verify(ctx context.Context, t Tool, dir string) (map[string]string, error) {
	result := map[string]string{}
	for _, exe := range executables(t) {
		output, err := v.run(ctx, filepath.Join(dir, exe), t == FFmpeg)
		if err != nil {
			return nil, err
		}
		version, err := parseVersion(t, exe, output)
		if err != nil {
			if toolErr, ok := err.(*ToolError); ok {
				toolErr.Detail += " stdout=" + output
			}
			return nil, err
		}
		result[exe] = version
	}
	if t == FFmpeg && result["ffmpeg.exe"] != result["ffprobe.exe"] {
		return nil, failure("version", "Версии FFmpeg и ffprobe не согласованы", "version mismatch")
	}
	return result, nil
}
func (v ProcessVerifier) run(ctx context.Context, exe string, ffmpeg bool) (string, error) {
	outRead, out, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer outRead.Close()
	defer out.Close()
	errRead, stderr, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer stderr.Close()
	defer errRead.Close()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	type capture struct {
		text string
		err  error
	}
	read := func(r *os.File, ch chan capture) {
		var b bytes.Buffer
		_, e := io.CopyN(&b, r, (64<<10)+1)
		if b.Len() > 64<<10 {
			cancel()
			ch <- capture{b.String()[:64<<10], fmt.Errorf("Превышен лимит вывода version command")}
			return
		}
		if e == io.EOF {
			e = nil
		}
		ch <- capture{b.String(), e}
	}
	outDone, errDone := make(chan capture, 1), make(chan capture, 1)
	go read(outRead, outDone)
	go read(errRead, errDone)
	args := []string{"--version"}
	if ffmpeg {
		args = []string{"-version"}
	}
	code, runErr := process.RunInJob(ctx, exe, args, filepath.Dir(exe), out, stderr)
	out.Close()
	stderr.Close()
	b, raw := <-outDone, <-errDone
	if b.err != nil {
		runErr = b.err
	}
	if raw.err != nil {
		runErr = raw.err
	}
	if runErr != nil {
		return "", failure("executable", "Компонент не запускается. Требуется переустановка", fmt.Sprintf("exe=%s exit_code=%d error=%v stdout=%s stderr=%s", filepath.Base(exe), code, runErr, b.text, raw.text))
	}
	return b.text, nil
}
func parseVersion(t Tool, exe, output string) (string, error) {
	first := strings.TrimSpace(strings.SplitN(output, "\n", 2)[0])
	version := first
	switch t {
	case YtDlp:
		if !regexp.MustCompile(`^\d{4}\.\d{2}\.\d{2}$`).MatchString(first) {
			return "", failure("version", "Не удалось определить версию yt-dlp", "unexpected version output")
		}
	case FFmpeg:
		fields := strings.Fields(first)
		if len(fields) < 3 || fields[0] != strings.TrimSuffix(exe, ".exe") || fields[1] != "version" {
			return "", failure("version", "Не удалось определить версию FFmpeg / ffprobe", "unexpected version output")
		}
		version = fields[2]
	case Deno:
		fields := strings.Fields(first)
		if len(fields) < 2 || fields[0] != "deno" {
			return "", failure("version", "Не удалось определить версию Deno", "unexpected version output")
		}
		version = fields[1]
		if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
			return "", failure("version", "Некорректная версия Deno", "invalid semver")
		}
		parts := strings.Split(version, ".")
		if len(parts) != 3 {
			return "", fmt.Errorf("Некорректная версия Deno")
		}
		major, e1 := strconv.Atoi(parts[0])
		minor, e2 := strconv.Atoi(parts[1])
		_, e3 := strconv.Atoi(parts[2])
		if e1 != nil || e2 != nil || e3 != nil || major < 2 || (major == 2 && minor < 3) {
			return "", failure("version", "Для yt-dlp требуется Deno 2.3.0 или новее", "unsupported Deno")
		}
	}
	if len(version) > 200 {
		return "", fmt.Errorf("Слишком длинная версия компонента")
	}
	return version, nil
}
