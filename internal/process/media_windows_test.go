//go:build windows

package process

import (
	"YTDLP-GUI-Mangar/internal/testfixture"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Opt-in integration использует установленные tools и только локальный HTTP.
// Никакие пользовательские URL/видео не используются.
func TestRealYtdlpFFmpegCancellation(t *testing.T) {
	if os.Getenv("MANGAR_REAL_TOOLS") != "1" {
		t.Skip("MANGAR_REAL_TOOLS=1 включает локальный yt-dlp → FFmpeg smoke")
	}
	executables := testfixture.Executables(t, testfixture.Tools(t))
	ytdlp, ffmpeg := executables["yt-dlp.exe"], executables["ffmpeg.exe"]
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	fixture := filepath.Join(dir, "fixture.mp4")
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=size=160x90:rate=10", "-t", "60", "-c:v", "libx264", "-movflags", "+faststart", fixture)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, fixture) }))
	defer server.Close()
	out, errout := outputs(t, dir)
	exe, _ := os.Executable()
	done := make(chan error, 1)
	go func() {
		_, err := RunInJob(ctx, exe, append(helperArgs("media", dir), ytdlp, server.URL+"/video.mp4"), dir, out, errout)
		done <- err
	}()
	root := pidHandle(t, dir, "media")
	rootPID := waitForPIDFile(filepath.Join(dir, "media.pid"))
	var children []windows.Handle
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		entries := processSnapshot(t)
		descendants := map[uint32]bool{uint32(rootPID): true}
		for range 10 {
			for _, entry := range entries {
				if descendants[entry.ParentProcessID] {
					descendants[entry.ProcessID] = true
				}
			}
		}
		foundFFmpeg := false
		for _, entry := range entries {
			if descendants[entry.ProcessID] && strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "ffmpeg.exe") {
				foundFFmpeg = true
			}
		}
		if foundFFmpeg {
			for _, entry := range entries {
				if descendants[entry.ProcessID] && entry.ProcessID != uint32(rootPID) {
					h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, entry.ProcessID)
					if err != nil {
						t.Fatal(err)
					}
					children = append(children, h)
					t.Cleanup(func() { windows.TerminateProcess(h, 99); windows.CloseHandle(h) })
					t.Logf("Реальный потомок: %s", windows.UTF16ToString(entry.ExeFile[:]))
				}
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		raw, _ := os.ReadFile(errout.Name())
		t.Fatalf("%v\n%s", err, raw)
	}
	assertStopped(t, append(children, root)...)
	if len(children) == 0 {
		raw, _ := os.ReadFile(errout.Name())
		t.Fatalf("FFmpeg не наблюдался\n%s", raw)
	}
	// Удаление возможно только после подтверждённого завершения job.
	out.Close()
	errout.Close()
	for _, name := range []string{"result.mp4", "result.mp4.part"} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
}

func processSnapshot(t *testing.T) []windows.ProcessEntry32 {
	t.Helper()
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	var entries []windows.ProcessEntry32
	for err := windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		entries = append(entries, entry)
	}
	return entries
}
