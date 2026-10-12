package download

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestFilenameSecretIDsAndWindowsNames(t *testing.T) {
	for _, id := range []string{"video?ticket=SYNTHETIC_SECRET", "https://cdn.test/v?sig=SYNTHETIC_SECRET", "token%253DSYNTHETIC_SECRET", "SYNTHETIC_SECRET", "CON", "NUL", "COM1", "LPT9", "\x00\x7f\u202e"} {
		name := safeName("Название 日本語 & (тест)", id, 80)
		if strings.Contains(name, "SYNTHETIC_SECRET") || strings.ContainsAny(name, `<>:"/\|?*`) || !strings.Contains(name, "日本語") || len(utf16.Encode([]rune(name))) > 80 {
			t.Fatal(name)
		}
	}
	for _, title := range []string{"video?ticket=SYNTHETIC_SECRET", "password: SYNTHETIC_SECRET", "https://cdn.test/SYNTHETIC_SECRET", "v%3Ftoken%3DSYNTHETIC_SECRET"} {
		if name := safeName(title, "safe", 80); strings.Contains(name, "SYNTHETIC_SECRET") {
			t.Fatal(name)
		}
	}
	for _, title := range []string{"CON", "CON.txt", "PRN", "AUX", "COM¹", "NUL. ", strings.Repeat("🎬", 300)} {
		name := safeName(title, "id", 80)
		if len(utf16.Encode([]rune(name))) > 80 || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
			t.Fatal(name)
		}
	}
}

func TestSecretMetadataNeverReachesNameSnapshotOrLog(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	base := m.run
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		return base(ctx, exe, args, dir, func(stream, text string) {
			if strings.HasPrefix(text, metaPrefix) {
				text = metaPrefix + `{"id":"video?ticket=SYNTHETIC_SECRET","title":"Фильм 日本語"}`
			}
			line(stream, text)
		})
	}
	if _, err := m.Start(Request{URL: "https://host/v?ticket=SYNTHETIC_SECRET", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, m)
	if job.State != "completed" || strings.Contains(job.FinalPath, "SYNTHETIC_SECRET") || !strings.Contains(job.FinalPath, "Фильм") {
		t.Fatal(job)
	}
	files, _ := filepath.Glob(filepath.Join(m.paths.Logs, "jobs", "*.log"))
	for _, file := range files {
		b, _ := os.ReadFile(file)
		if strings.Contains(string(b), "SYNTHETIC_SECRET") {
			t.Fatal("secret log leak")
		}
	}
}

func TestBareQuerySecretTitleAndReservedPublication(t *testing.T) {
	if !sourceSecretTitle(strings.Repeat("a", 600), "https://host/video?ticket="+strings.Repeat("a", 600)) {
		t.Fatal("long query title")
	}
	for _, tc := range []struct{ title, source string }{{"SYN\u202eTHETIC_SECRET", "https://host/video?ticket=SYNTHETIC_SECRET"}, {"abc%2FSECRET", "https://host/video?ticket=abc%2FSECRET"}} {
		if !sourceSecretTitle(tc.title, tc.source) {
			t.Fatal("normalized secret title")
		}
	}
	if title, _ := browserTitle("SYNTHETIC_SECRET", Request{URL: "https://host/video?ticket=SYNTHETIC_SECRET"}); strings.Contains(title, "SYNTHETIC_SECRET") {
		t.Fatal("query title leak")
	}
	if title, _ := browserTitle("master", Request{URL: "https://host/master.m3u8", Browser: &BrowserContext{PageTitle: "PAGE_SECRET", PageURL: "https://page.test?ticket=PAGE_SECRET"}}); strings.Contains(title, "PAGE_SECRET") {
		t.Fatal("page title leak")
	}
	root := t.TempDir()
	src := filepath.Join(root, "source")
	os.WriteFile(src, []byte("fixture"), 0600)
	for _, title := range []string{"CON.txt", "NUL.mp4", "COM1.mp4", "LPT².txt", "CONIN$.mp4", "CONOUT$.txt"} {
		path, err := publish(context.Background(), src, root, title, "id", "publication")
		if err != nil {
			t.Fatal(title, err)
		}
		if stat, err := os.Stat(path); err != nil || !stat.Mode().IsRegular() {
			t.Fatal(title, err)
		}
	}
}

func TestFFmpegDiskFullIsShortAndNeverTrialsOrEncodes(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	base := m.run
	calls := 0
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "ffmpeg.exe" {
			calls++
			line("stderr", "Error writing trailer: No space left on device")
			return 1, errors.New("external exit 1")
		}
		return base(ctx, exe, args, dir, line)
	}
	if _, err := m.Start(Request{URL: "https://host/video", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	if job := waitJob(t, m); job.State != "failed" || job.Error != "Недостаточно места на диске" || calls != 1 {
		t.Fatal(job.State, job.Error, calls)
	}
}

func TestMalformedMetadataCannotPanicOrReplaceValidTitle(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	base := m.run
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		return base(ctx, exe, args, dir, func(stream, text string) {
			line(stream, text)
			if strings.HasPrefix(text, metaPrefix) {
				for _, value := range []string{"null", "[]", `"SECRET"`, "invalid"} {
					line("stdout", metaPrefix+value)
				}
			}
		})
	}
	if _, err := m.Start(Request{URL: "https://host/video", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	if job := waitJob(t, m); job.State != "completed" || job.Title != "Тест & Unicode 日本語" {
		t.Fatal(job.State, job.Title)
	}
}

func TestBareQuerySecretMetadataEndToEnd(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	base := m.run
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		return base(ctx, exe, args, dir, func(stream, text string) {
			if strings.HasPrefix(text, metaPrefix) {
				text = metaPrefix + `{"id":"SYNTHETIC_SECRET","title":"SYNTHETIC_SECRET"}`
			}
			line(stream, text)
		})
	}
	if _, err := m.Start(Request{URL: "https://host/video?ticket=SYNTHETIC_SECRET", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, m)
	if job.State != "completed" || strings.Contains(job.Title+job.FinalPath+job.Message, "SYNTHETIC_SECRET") {
		t.Fatal("metadata leak", job.State)
	}
}

func TestNetworkAndFilesystemErrorCopy(t *testing.T) {
	for _, raw := range []string{"The read operation timed out", "TLS handshake timeout", "Connection reset by peer", "getaddrinfo failed", "network is unreachable"} {
		if httpFailureMessage(raw) != networkFailure {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"Traceback: FileNotFoundError PRIVATE", "File 403.mp4 not found"} {
		if httpFailureMessage(raw) != "" {
			t.Fatal(raw)
		}
	}
	if filesystemFailure(errors.Join(errors.New("private path"), windows.ERROR_DISK_FULL)) != "Недостаточно места на диске" {
		t.Fatal("disk error")
	}
	for _, raw := range []string{"Cookie: SECRET\nAuthorization: Bearer SECRET", "X-Playback-Secret: SECRET", "https://googlevideo.test/private?expire=123&sig=SECRET", `{"signature":"SECRET"}`} {
		if strings.Contains(redact(raw), "SECRET") {
			t.Fatal("redaction leak")
		}
	}
}

func TestNetworkRetryWarningThenProgressRecovers(t *testing.T) {
	m, dest, _ := fixtureManager(t, "")
	base := m.run
	observed := false
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if filepath.Base(exe) == "yt-dlp.exe" {
			line("stderr", "TLS handshake timeout. Retrying (1/3)")
			observed = strings.Contains(lastJob(m).Message, "Повторяем")
		}
		return base(ctx, exe, args, dir, line)
	}
	if _, err := m.Start(Request{URL: "https://host/video", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	if job := waitJob(t, m); job.State != "completed" || !observed {
		t.Fatal(job, observed)
	}
}
