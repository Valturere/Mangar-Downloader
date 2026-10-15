package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func gcFixture(t *testing.T) (*fixture, Manifest) {
	t.Helper()
	f := newFixture(t)
	if err := f.m.InstallNeeded(context.Background()); err != nil {
		t.Fatal(err)
	}
	man := f.m.active[YtDlp]
	source := f.m.versionDir(YtDlp, man.Identity)
	man.Identity = "r1-a1-0000000000000001"
	dest := f.m.versionDir(YtDlp, man.Identity)
	if err := os.CopyFS(dest, os.DirFS(source)); err != nil {
		t.Fatal(err)
	}
	return f, man
}

func gcPointer(t *testing.T, m *Manager, name string, man Manifest) {
	t.Helper()
	if err := writeManifest(filepath.Join(m.toolDir(man.Tool), name), man, m.replace); err != nil {
		t.Fatal(err)
	}
}

func assertGCVersion(t *testing.T, m *Manager, man Manifest, exists bool) {
	t.Helper()
	for name := range man.Files {
		_, err := os.Stat(filepath.Join(m.versionDir(man.Tool, man.Identity), name))
		if (exists && err != nil) || (!exists && !os.IsNotExist(err)) {
			t.Fatalf("identity=%s file=%s expected exists=%t: %v", man.Identity, name, exists, err)
		}
	}
}

func TestGCProtectsCurrentPreviousAndLease(t *testing.T) {
	for _, protected := range []string{"current.json", "previous.json", "lease", "orphan"} {
		t.Run(protected, func(t *testing.T) {
			f, candidate := gcFixture(t)
			if strings.HasSuffix(protected, ".json") {
				gcPointer(t, f.m, protected, candidate)
			} else if protected == "lease" {
				f.m.active[YtDlp] = candidate // Stale active view, no pointer to candidate.
				lease, err := f.m.Acquire(context.Background(), YtDlp)
				if err != nil {
					t.Fatal(err)
				}
				defer lease.Close()
			}
			if err := f.m.Detect(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertGCVersion(t, f.m, candidate, protected != "orphan")
			assertGCVersion(t, f.m, f.m.active[YtDlp], true)
		})
	}
}

func TestGCRechecksPointersAndLeasesAfterEnumeration(t *testing.T) {
	for _, protected := range []string{"current.json", "previous.json", "lease"} {
		t.Run(protected, func(t *testing.T) {
			f, candidate := gcFixture(t)
			end, err := f.m.begin()
			if err != nil {
				t.Fatal(err)
			}
			defer end()
			// Enumerate an actual orphan, then change the protection before the
			// final candidate deletion. The initial keep-set would miss it.
			current, previous, err := f.m.cleanupPointers(YtDlp)
			if err != nil || candidate.Identity == current || candidate.Identity == previous {
				t.Fatal("candidate is not initially orphaned", err)
			}
			entries, err := os.ReadDir(filepath.Join(f.m.toolDir(YtDlp), "versions"))
			if err != nil {
				t.Fatal(err)
			}
			if protected == "lease" {
				f.m.mu.Lock()
				f.m.leases[candidate.Identity]++
				f.m.mu.Unlock()
			} else {
				gcPointer(t, f.m, protected, candidate)
			}
			for _, entry := range entries {
				if entry.Name() == candidate.Identity {
					f.m.cleanVersion(YtDlp, entry.Name())
				}
			}
			assertGCVersion(t, f.m, candidate, true)
		})
	}
}

func TestGCManifestErrorsPreserveCandidates(t *testing.T) {
	for _, pointer := range []string{"current.json", "previous.json"} {
		for _, corrupt := range []string{"malformed", "directory", "locked"} {
			for _, final := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/final=%t", pointer, corrupt, final), func(t *testing.T) {
					f, candidate := gcFixture(t)
					end, err := f.m.begin()
					if err != nil {
						t.Fatal(err)
					}
					defer end()
					p := filepath.Join(f.m.toolDir(YtDlp), pointer)
					if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
						t.Fatal(err)
					}
					if corrupt == "locked" {
						gcPointer(t, f.m, pointer, candidate)
						ptr, _ := windows.UTF16PtrFromString(p)
						h, lockErr := windows.CreateFile(ptr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
						if lockErr != nil {
							t.Fatal(lockErr)
						}
						defer windows.CloseHandle(h)
					} else if corrupt == "directory" {
						err = os.Mkdir(p, 0700)
					} else {
						err = os.WriteFile(p, []byte("{"), 0600)
					}
					if err != nil {
						t.Fatal(err)
					}
					if final {
						f.m.cleanVersion(YtDlp, candidate.Identity)
					} else {
						f.m.cleanVersions(YtDlp)
					}
					assertGCVersion(t, f.m, candidate, true)
				})
			}
		}
	}
}

func TestGCAuditAndMissingPointers(t *testing.T) {
	f, candidate := gcFixture(t)
	if err := os.Remove(filepath.Join(f.m.toolDir(YtDlp), "current.json")); err != nil {
		t.Fatal(err)
	}
	var events []string
	f.m.log = func(tool Tool, phase, detail string) {
		if tool != YtDlp {
			t.Fatal("wrong audit tool", tool)
		}
		if strings.HasPrefix(phase, "gc_") && strings.Contains(detail, "identity="+candidate.Identity+" ") {
			if phase == "gc_delete" {
				assertGCVersion(t, f.m, candidate, true)
			} else if phase == "gc_deleted" {
				assertGCVersion(t, f.m, candidate, false)
			}
			events = append(events, phase+" "+detail)
		}
	}
	end, err := f.m.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	f.m.cleanVersions(YtDlp)
	assertGCVersion(t, f.m, candidate, false)
	if len(events) != 2 || events[0] != "gc_delete identity="+candidate.Identity+" reason=orphan current= previous= leased=false" || events[1] != "gc_deleted identity="+candidate.Identity+" result=deleted" {
		t.Fatal(events)
	}
}

func TestGCLeaseAcrossManagersAndConcurrentAcquire(t *testing.T) {
	f, candidate := gcFixture(t)
	other := New(Options{Paths: f.m.paths, Verifier: f.m.verifier})
	other.active[YtDlp] = candidate
	lease, err := other.Acquire(context.Background(), YtDlp)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := f.m.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGCVersion(t, f.m, candidate, true)
	lease.Close()
	// Once GC owns DELETE access, a stale Manager must fail to acquire a
	// lease; it must never return paths to a partially deleted version.
	unpin, err := f.m.guardVersion(YtDlp, candidate.Identity, true)
	if err != nil {
		t.Fatal(err)
	}
	got, err := other.Acquire(context.Background(), YtDlp)
	unpin()
	if got != nil || err == nil {
		t.Fatal("lease crossed GC boundary", got, err)
	}
	if err := f.m.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGCVersion(t, f.m, candidate, false)
}

func TestGCConcurrentLocalLeases(t *testing.T) {
	f, candidate := gcFixture(t)
	f.m.active[YtDlp] = candidate
	end, err := f.m.begin()
	if err != nil {
		t.Fatal(err)
	}
	defer end()
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < 30; i++ {
			if lease, err := f.m.Acquire(context.Background(), YtDlp); err == nil {
				assertGCVersion(t, f.m, candidate, true)
				lease.Close()
			}
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < 30; i++ {
			f.m.cleanVersion(YtDlp, candidate.Identity)
		}
	}()
	workers.Wait()
}

func TestGCFailedAcquireReleasesGuard(t *testing.T) {
	f, candidate := gcFixture(t)
	f.m.active[YtDlp] = candidate
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, err := f.m.Acquire(ctx, YtDlp); err == nil || lease != nil {
		t.Fatal("cancelled Acquire returned a lease", lease, err)
	}
	if err := f.m.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGCVersion(t, f.m, candidate, false)
}

func TestGCLeaseProcess(t *testing.T) {
	if path := os.Getenv("MANGAR_GC_LEASE_TEST_PATH"); path != "" {
		unpin, err := versionGuard(path, false)
		if err != nil {
			t.Fatal(err)
		}
		defer unpin()
		fmt.Println("LEASE_READY")
		var release string
		fmt.Scanln(&release)
		return
	}
	f, candidate := gcFixture(t)
	// Prepare the sidecar with a normal lease before the child opens it.
	unpin, err := f.m.guardVersion(YtDlp, candidate.Identity, false)
	if err != nil {
		t.Fatal(err)
	}
	unpin()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestGCLeaseProcess$")
	cmd.Env = append(os.Environ(), "MANGAR_GC_LEASE_TEST_PATH="+filepath.Join(f.m.paths.Tools, ".leases", string(YtDlp), candidate.Identity+".lock"))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stdin.Close(); cmd.Wait() })
	ready := make([]byte, len("LEASE_READY\n"))
	if _, err := io.ReadFull(stdout, ready); err != nil || strings.TrimSpace(string(ready)) != "LEASE_READY" {
		t.Fatal("lease helper failed", string(ready), err)
	}
	if err := f.m.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGCVersion(t, f.m, candidate, true)
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Detect(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertGCVersion(t, f.m, candidate, false)
}
