package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLeaseAndRollback(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	lease, e := f.m.Acquire(ctx, YtDlp)
	if e != nil {
		t.Fatal(e)
	}
	oldPath := lease.Paths["yt-dlp.exe"]
	for _, v := range []int{2, 3} {
		f.version = v
		if e = f.m.CheckUpdates(ctx); e != nil {
			t.Fatal(e)
		}
		if e = f.m.InstallNeeded(ctx); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = os.Stat(oldPath); e != nil {
		t.Fatal("leased version deleted", e)
	}
	if e = f.m.Rollback(ctx, YtDlp); e != nil {
		t.Fatal(e)
	}
	if f.m.active[YtDlp].ReleaseID != 2 {
		t.Fatal("rollback failed")
	}
	lease.Close()
	lease.Close()
	f.m.cleanVersions(YtDlp)
	if _, e = os.Stat(oldPath); !os.IsNotExist(e) {
		t.Fatal("unleased old version not collected")
	}
}
func TestLockedObsoletePairIsPreserved(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	old := f.m.active[FFmpeg]
	dir := f.m.versionDir(FFmpeg, old.Identity)
	p, _ := windows.UTF16PtrFromString(filepath.Join(dir, "ffmpeg.exe"))
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer windows.CloseHandle(h)
	for _, v := range []int{2, 3} {
		f.version = v
		if e = f.m.CheckUpdates(ctx); e != nil {
			t.Fatal(e)
		}
		if e = f.m.InstallNeeded(ctx); e != nil {
			t.Fatal(e)
		}
	}
	for _, exe := range executables(FFmpeg) {
		if _, e = os.Stat(filepath.Join(dir, exe)); e != nil {
			t.Fatal("locked pair partially removed", e)
		}
	}
	windows.CloseHandle(h)
	f.m.cleanVersions(FFmpeg)
	if _, e = os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("unlocked old version retained")
	}
}
func TestDiskFullDuringStreaming(t *testing.T) {
	f := newFixture(t)
	f.m.http.Write = func(*os.File, []byte) (int, error) { return 0, windows.ERROR_DISK_FULL }
	if e := f.m.InstallNeeded(context.Background()); e == nil {
		t.Fatal("disk full ignored")
	}
	if f.m.Snapshot().Tools[0].Message != "Недостаточно места на диске для установки компонентов" {
		t.Fatal("disk full user message unclear")
	}
	if _, ok := f.m.active[YtDlp]; ok {
		t.Fatal("disk full activated tool")
	}
	entries, _ := os.ReadDir(filepath.Join(f.m.paths.Tools, ".staging"))
	if len(entries) != 0 {
		t.Fatal("disk full partial remains")
	}
}
func TestDenoPowerShellChecksumRegression(t *testing.T) {
	sum := strings.Repeat("a", 64)
	b := "\r\nAlgorithm : SHA256\r\nHash      : " + strings.ToUpper(sum) + "\r\nPath      : C:\\a\\deno\\target\\release\\deno-x86_64-pc-windows-msvc.zip\r\n\r\n"
	got, e := checksumFor([]byte(b), "deno-x86_64-pc-windows-msvc.zip")
	if e != nil || got != sum {
		t.Fatal(got, e)
	}
	if _, e = checksumFor([]byte(b), "other.zip"); e == nil {
		t.Fatal("wrong asset accepted")
	}
}
func TestLoggerSecretsAndRotation(t *testing.T) {
	dir := t.TempDir()
	log := ToolLogger(dir)
	log(Deno, "network", "https://release-assets.githubusercontent.com/path?token=secret#frag raw stderr")
	p := filepath.Join(dir, "tools.jsonl")
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "secret") || !strings.Contains(string(b), "raw stderr") {
		t.Fatal(string(b))
	}
	os.WriteFile(p, make([]byte, 2<<20), 0600)
	log(Deno, "test", "diagnostic")
	if _, e := os.Stat(p + ".1"); e != nil {
		t.Fatal(e)
	}
}
func TestReparseCleanupRefused(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep")
	os.WriteFile(keep, []byte("user data"), 0600)
	os.MkdirAll(f.m.paths.Tools, 0700)
	link := filepath.Join(f.m.paths.Tools, ".staging")
	if e := os.Symlink(outside, link); e != nil {
		t.Skip("symlink not available:", e)
	}
	if e := f.m.removeOwned(link); e == nil {
		t.Fatal("reparse point removed")
	}
	if _, e := os.Stat(keep); e != nil {
		t.Fatal("outside deleted")
	}
}
func TestCancellationDuringInstallLeavesOldCurrent(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.verifyFailure = func(tool Tool, dir string) error {
		if tool == YtDlp {
			cancel()
			return context.Canceled
		}
		return nil
	}
	if e := f.m.InstallNeeded(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(f.m.toolDir(YtDlp), "current.json")); !os.IsNotExist(e) {
		t.Fatal("cancel activated version")
	}
	entries, _ := os.ReadDir(filepath.Join(f.m.paths.Tools, ".staging"))
	if len(entries) != 0 {
		t.Fatal("cancel cleanup")
	}
}
