package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"golang.org/x/sys/windows"
)

type sourceFunc func(context.Context, Tool) (Candidate, error)

func (f sourceFunc) Latest(ctx context.Context, t Tool) (Candidate, error) { return f(ctx, t) }

type verifierFunc func(context.Context, Tool, string) (map[string]string, error)

func (f verifierFunc) Verify(ctx context.Context, t Tool, dir string) (map[string]string, error) {
	return f(ctx, t, dir)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixture struct {
	m             *Manager
	server        *httptest.Server
	version       int
	requests      map[Tool]int
	mu            sync.Mutex
	verifyFailure func(Tool, string) error
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{version: 1, requests: map[Tool]int{}}
	artifacts := map[Tool][]byte{YtDlp: []byte("fake exe"), FFmpeg: zipBytes(t, []string{"build/bin/ffmpeg.exe", "build/bin/ffprobe.exe", "build/LICENSE.txt"}), Deno: zipBytes(t, []string{"deno.exe"})}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tool := Tool(strings.TrimPrefix(r.URL.Path, "/"))
		f.mu.Lock()
		f.requests[tool]++
		f.mu.Unlock()
		w.Write(artifacts[tool])
	}))
	t.Cleanup(f.server.Close)
	h := testHTTP(f.server)
	transport := h.Client.Transport
	h.Client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "raw.githubusercontent.com" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("fixture license")), Header: make(http.Header)}, nil
		}
		return transport.RoundTrip(r)
	})
	source := sourceFunc(func(ctx context.Context, tool Tool) (Candidate, error) {
		if err := ctx.Err(); err != nil {
			return Candidate{}, err
		}
		tag := fmt.Sprintf("2026.08.%02d", f.version)
		if tool == Deno {
			tag = fmt.Sprintf("v2.9.%d", f.version)
		}
		a := Asset{ID: int64(f.version), Name: string(tool), Size: int64(len(artifacts[tool])), URL: f.server.URL + "/" + string(tool), Digest: "sha256:" + digest(artifacts[tool])}
		return Candidate{Tool: tool, Repository: repository(tool), ReleaseID: int64(f.version), Tag: tag, Asset: a}, nil
	})
	verifier := verifierFunc(func(ctx context.Context, tool Tool, dir string) (map[string]string, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if f.verifyFailure != nil {
			if e := f.verifyFailure(tool, dir); e != nil {
				return nil, e
			}
		}
		versions := map[string]string{}
		versionIndex := f.version
		var installedIndex int
		if n, _ := fmt.Sscanf(filepath.Base(dir), "r%d-", &installedIndex); n == 1 {
			versionIndex = installedIndex
		}
		for _, exe := range executables(tool) {
			if _, e := os.Stat(filepath.Join(dir, exe)); e != nil {
				return nil, e
			}
			v := fmt.Sprintf("2026.08.%02d", versionIndex)
			if tool == Deno {
				v = fmt.Sprintf("2.9.%d", versionIndex)
			}
			if tool == FFmpeg {
				v = "ffmpeg-fixture"
			}
			versions[exe] = v
		}
		return versions, nil
	})
	paths := apppaths.At(filepath.Join(t.TempDir(), "Кириллица пробелы & 100%"))
	f.m = New(Options{Paths: paths, HTTP: h, Source: source, Verifier: verifier})
	return f
}
func assertInstalled(t *testing.T, m *Manager) {
	t.Helper()
	for _, s := range m.Snapshot().Tools {
		if s.State != "installed" {
			t.Fatalf("not installed: %+v", s)
		}
	}
}
func TestSequentialInstallLocalDetectionResolverAndRetry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if e := f.m.Detect(ctx); e != nil {
		t.Fatal(e)
	}
	for _, s := range f.m.Snapshot().Tools {
		if s.State != "not_installed" {
			t.Fatal(s)
		}
	}
	f.verifyFailure = func(tool Tool, dir string) error {
		if tool == FFmpeg {
			return errors.New("injected executable failure")
		}
		return nil
	}
	if e := f.m.InstallNeeded(ctx); e == nil {
		t.Fatal("expected failure")
	}
	if f.requests[YtDlp] != 1 || f.requests[Deno] != 0 {
		t.Fatal(f.requests)
	}
	f.verifyFailure = nil
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, f.m)
	if f.requests[YtDlp] != 1 {
		t.Fatal("working yt-dlp downloaded again")
	}
	if _, e := f.m.ResolveYtDlp(ctx); e != nil {
		t.Fatal(e)
	}
	if _, _, e := f.m.ResolveFFmpegPair(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := f.m.ResolveDeno(ctx); e != nil {
		t.Fatal(e)
	}
	f.m.source = sourceFunc(func(context.Context, Tool) (Candidate, error) {
		t.Fatal("detection uses network")
		return Candidate{}, nil
	})
	if e := f.m.Detect(ctx); e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, f.m)
	path, e := f.m.ResolveDeno(ctx)
	if e != nil {
		t.Fatal(e)
	}
	os.Remove(path)
	if _, e = f.m.ResolveDeno(ctx); e == nil {
		t.Fatal("deleted exe resolved")
	}
	if e = f.m.Detect(ctx); e != nil {
		t.Fatal(e)
	}
	if f.m.Snapshot().Tools[2].State != "corrupted" {
		t.Fatal("deleted Deno not corrupted")
	}
}
func TestManifestAndInterruptedInstall(t *testing.T) {
	for _, mode := range []string{"malformed", "missing version", "traversal", "missing probe", "corrupt executable", "interrupted"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			if e := f.m.InstallNeeded(ctx); e != nil {
				t.Fatal(e)
			}
			man := f.m.active[FFmpeg]
			current := filepath.Join(f.m.toolDir(FFmpeg), "current.json")
			switch mode {
			case "malformed":
				os.WriteFile(current, []byte("invalid JSON"), 0600)
			case "missing version":
				os.RemoveAll(f.m.versionDir(FFmpeg, man.Identity))
			case "traversal":
				man.Identity = "../outside"
				writeManifest(current, man, replaceFile)
			case "missing probe":
				os.Remove(filepath.Join(f.m.versionDir(FFmpeg, man.Identity), "ffprobe.exe"))
			case "corrupt executable":
				os.WriteFile(filepath.Join(f.m.versionDir(FFmpeg, man.Identity), "ffmpeg.exe"), []byte("changed"), 0600)
			case "interrupted":
				stage := filepath.Join(f.m.paths.Tools, ".staging", "install-interrupted")
				os.MkdirAll(stage, 0700)
				os.WriteFile(filepath.Join(stage, "partial"), []byte("partial"), 0600)
			}
			before := len(f.requests)
			if e := f.m.Detect(ctx); e != nil {
				t.Fatal(e)
			}
			if len(f.requests) != before {
				t.Fatal("startup network")
			}
			state := f.m.Snapshot().Tools[1].State
			if mode == "interrupted" {
				if state != "installed" {
					t.Fatal(state)
				}
				entries, _ := os.ReadDir(filepath.Join(f.m.paths.Tools, ".staging"))
				if len(entries) != 0 {
					t.Fatal("interrupted staging not cleaned")
				}
			} else if state != "corrupted" {
				t.Fatal(state)
			}
		})
	}
}
func TestUpdateFailurePreservesCurrentAndActivationRollback(t *testing.T) {
	for _, mode := range []string{"download", "verify", "rename", "manifest write", "destination locked", "post activation"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			if e := f.m.InstallNeeded(ctx); e != nil {
				t.Fatal(e)
			}
			old := f.m.active[YtDlp]
			current := filepath.Join(f.m.toolDir(YtDlp), "current.json")
			oldBytes, _ := os.ReadFile(current)
			f.version = 2
			if e := f.m.CheckUpdates(ctx); e != nil {
				t.Fatal(e)
			}
			switch mode {
			case "download":
				f.m.http.Create = func(string) (*os.File, error) { return nil, errors.New("injected disk full") }
			case "verify":
				f.verifyFailure = func(_ Tool, dir string) error {
					if filepath.Base(dir) != old.Identity {
						return errors.New("version command failed")
					}
					return nil
				}
			case "rename":
				f.m.rename = func(string, string) error { return errors.New("injected access denied") }
			case "manifest write":
				f.m.replace = func(a, b string) error {
					if b == current {
						return errors.New("injected write failure")
					}
					return replaceFile(a, b)
				}
			case "destination locked":
				p, _ := windows.UTF16PtrFromString(current)
				handle, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
				if e != nil {
					t.Fatal(e)
				}
				defer windows.CloseHandle(handle)
			case "post activation":
				f.verifyFailure = func(tool Tool, dir string) error {
					if tool == YtDlp {
						active, e := readManifest(current, YtDlp)
						if e == nil && active.Identity != old.Identity {
							return errors.New("injected post-activation failure")
						}
					}
					return nil
				}
			}
			if e := f.m.InstallNeeded(ctx); e == nil {
				t.Fatal("failure ignored")
			}
			now, e := os.ReadFile(current)
			if e != nil {
				t.Fatal(e)
			}
			if string(now) != string(oldBytes) {
				t.Fatal("old current damaged")
			}
			if f.m.active[YtDlp].Identity != old.Identity {
				t.Fatal("active changed")
			}
			entries, _ := os.ReadDir(filepath.Join(f.m.paths.Tools, ".staging"))
			if len(entries) != 0 {
				t.Fatal("staging remains")
			}
			if _, e = os.Stat(filepath.Join(f.m.versionDir(YtDlp, old.Identity), "yt-dlp.exe")); e != nil {
				t.Fatal("old exe lost")
			}
		})
	}
}
func TestConcurrentOperationsAndShutdownCancellation(t *testing.T) {
	f := newFixture(t)
	entered := make(chan struct{})
	f.m.source = sourceFunc(func(ctx context.Context, t Tool) (Candidate, error) {
		close(entered)
		<-ctx.Done()
		return Candidate{}, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- f.m.InstallNeeded(ctx) }()
	<-entered
	if e := f.m.InstallNeeded(context.Background()); e == nil {
		t.Fatal("second install allowed")
	}
	if e := f.m.Detect(context.Background()); e == nil {
		t.Fatal("concurrent detection allowed")
	}
	second := New(Options{Paths: f.m.paths})
	if e := second.Detect(context.Background()); e == nil {
		t.Fatal("second instance allowed")
	}
	cancel()
	if e := <-done; e == nil {
		t.Fatal("cancel ignored")
	}
	if f.m.Snapshot().Busy {
		t.Fatal("busy remains")
	}
	if e := second.Detect(context.Background()); e != nil {
		t.Fatal("lock not released", e)
	}
}
func TestFilesystemFailures(t *testing.T) {
	for _, mode := range []string{"staging unavailable", "read only root", "permission denied injection"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			root := f.m.paths.Tools
			if mode == "read only root" {
				os.MkdirAll(filepath.Dir(root), 0700)
				os.WriteFile(root, []byte("file instead of directory"), 0600)
			} else {
				os.MkdirAll(root, 0700)
				if mode == "staging unavailable" {
					os.WriteFile(filepath.Join(root, ".staging"), []byte("file"), 0600)
				} else {
					f.m.http.Create = func(string) (*os.File, error) { return nil, os.ErrPermission }
				}
			}
			if e := f.m.InstallNeeded(context.Background()); e == nil {
				t.Fatal("filesystem failure ignored")
			}
		})
	}
}
func TestVersionsKeepCurrentPrevious(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	first := f.m.active[YtDlp].Identity
	for _, v := range []int{2, 3} {
		f.version = v
		if e := f.m.CheckUpdates(ctx); e != nil {
			t.Fatal(e)
		}
		if e := f.m.InstallNeeded(ctx); e != nil {
			t.Fatal(e)
		}
	}
	previous, e := readManifest(filepath.Join(f.m.toolDir(YtDlp), "previous.json"), YtDlp)
	if e != nil {
		t.Fatal(e)
	}
	if previous.ReleaseID != 2 {
		t.Fatal(previous)
	}
	if _, e = os.Stat(f.m.versionDir(YtDlp, first)); !os.IsNotExist(e) {
		t.Fatal("obsolete version retained")
	}
	entries, _ := os.ReadDir(filepath.Join(f.m.toolDir(YtDlp), "versions"))
	if len(entries) != 2 {
		t.Fatal(len(entries))
	}
}
