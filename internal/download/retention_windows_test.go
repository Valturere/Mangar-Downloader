package download

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionProvesStaleAndPreservesActiveUnknownAndDownloads(t *testing.T) {
	paths := apppaths.At(t.TempDir())
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour)
	makeTemp := func(id string, marked bool) string {
		dir := filepath.Join(paths.Temp, id)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, "media.part"), []byte("test"), 0600)
		if marked {
			os.WriteFile(filepath.Join(dir, ".mangar-owner"), []byte(ownerMarker), 0600)
			os.Chtimes(filepath.Join(dir, ".mangar-owner"), old, old)
		}
		return dir
	}
	stale := makeTemp("10000000000000000000000000000001", true)
	unknown := makeTemp("10000000-0000-0000-0000-000000000002", false)
	active := makeTemp("10000000-0000-0000-0000-000000000003", false)
	owner, err := tempOwner(active)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	stamp := windows.NsecToFiletime(old.UnixNano())
	if err := windows.SetFileTime(windows.Handle(owner.Fd()), nil, nil, &stamp); err != nil {
		t.Fatal(err)
	}
	recent := makeTemp("10000000-0000-0000-0000-000000000004", true)
	os.Chtimes(filepath.Join(recent, ".mangar-owner"), now, now)
	jobs := filepath.Join(paths.Logs, "jobs")
	os.MkdirAll(jobs, 0700)
	logPath := filepath.Join(jobs, "10000000-0000-0000-0000-000000000005.log")
	os.WriteFile(logPath, []byte("test"), 0600)
	os.Chtimes(logPath, old, old)
	log, err := newLog(paths.Logs, "10000000-0000-0000-0000-000000000006")
	if err != nil {
		t.Fatal(err)
	}
	defer log.close()
	if err := windows.SetFileTime(windows.Handle(log.file.Fd()), nil, nil, &stamp); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(paths.Root, "ready.mp4")
	os.WriteFile(ready, []byte("user"), 0600)
	diagnostic := filepath.Join(paths.Temp, "developer-diagnostic")
	os.MkdirAll(diagnostic, 0700)
	logs, temps := CleanupStale(paths, now)
	if logs != 1 || temps != 1 {
		t.Fatal(logs, temps)
	}
	for _, path := range []string{stale, logPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("stale retained", path, err)
		}
	}
	for _, path := range []string{unknown, active, recent, ready, diagnostic, log.file.Name()} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("protected removed", err)
		}
	}
	log.close()
	owner.Close()
	logs, temps = CleanupStale(paths, now)
	if logs != 1 || temps != 1 {
		t.Fatal("closed stale", logs, temps)
	}
}

func TestRetentionRejectsFakeMarkerAndLinkedTemp(t *testing.T) {
	paths := apppaths.At(t.TempDir())
	dir := filepath.Join(paths.Temp, "10000000-0000-0000-0000-000000000001")
	os.MkdirAll(dir, 0700)
	marker := filepath.Join(dir, ".mangar-owner")
	os.WriteFile(marker, []byte("foreign process"), 0600)
	old := time.Now().Add(-40 * 24 * time.Hour)
	os.Chtimes(marker, old, old)
	if logs, temps := CleanupStale(paths, time.Now()); logs+temps != 0 {
		t.Fatal("foreign deletion")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(paths.Root, "other-process")
	os.MkdirAll(outside, 0700)
	os.WriteFile(filepath.Join(outside, ".mangar-owner"), []byte(ownerMarker), 0600)
	os.Chtimes(filepath.Join(outside, ".mangar-owner"), old, old)
	link := filepath.Join(paths.Temp, "10000000000000000000000000000002")
	cmd := exec.Command("pwsh", "-NoProfile", "-Command", "New-Item -ItemType Junction -Path $env:MANGAR_TEST_LINK -Target $env:MANGAR_TEST_TARGET | Out-Null")
	cmd.Env = append(os.Environ(), "MANGAR_TEST_LINK="+link, "MANGAR_TEST_TARGET="+outside)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("junction fixture: %v %s", err, output)
	}
	if logs, temps := CleanupStale(paths, time.Now()); logs+temps != 0 {
		t.Fatal("junction deletion")
	}
	if _, err := os.Stat(filepath.Join(outside, ".mangar-owner")); err != nil {
		t.Fatal("foreign target removed", err)
	}
}
