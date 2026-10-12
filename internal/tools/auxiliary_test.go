package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRuntimeVsAuxiliaryHealthAndRepair(t *testing.T) {
	for _, name := range []string{"yt-dlp.exe", "LICENSE.txt", "PROVENANCE.txt", "ffprobe.exe"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			if err := f.m.InstallNeeded(ctx); err != nil {
				t.Fatal(err)
			}
			tool := YtDlp
			if name == "ffprobe.exe" {
				tool = FFmpeg
			}
			man := f.m.active[tool]
			dir := f.m.versionDir(tool, man.Identity)
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if err := f.m.Detect(ctx); err != nil {
				t.Fatal(err)
			}
			status := f.m.statuses[tool]
			aux := auxiliaryFile(name)
			if aux {
				if status.State != "installed" || !status.AuxiliaryRepair {
					t.Fatal(status)
				}
				lease, err := f.m.Acquire(ctx, tool)
				if err != nil {
					t.Fatal(err)
				}
				lease.Close()
			} else {
				if status.State != "corrupted" {
					t.Fatal(status)
				}
				if _, err := f.m.Acquire(ctx, tool); err == nil {
					t.Fatal("corrupt acquired")
				}
			}
			if err := f.m.InstallNeeded(ctx); err != nil {
				t.Fatal(err)
			}
			assertInstalled(t, f.m)
			if f.m.statuses[tool].AuxiliaryRepair {
				t.Fatal("repair failed")
			}
			if f.m.active[tool].Identity == man.Identity {
				t.Fatal("repair mutated immutable version")
			}
			for _, sibling := range toolOrder {
				if lease, err := f.m.Acquire(ctx, sibling); err != nil {
					t.Fatal("sibling broken", err)
				} else {
					lease.Close()
				}
			}
		})
	}
}
func TestDetectAndCleanupKeepCurrentDirectoryUnchanged(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.m.InstallNeeded(ctx); err != nil {
		t.Fatal(err)
	}
	before := map[Tool]map[string]string{}
	identities := map[Tool]string{}
	for _, tool := range toolOrder {
		man := f.m.active[tool]
		identities[tool] = man.Identity
		files, err := hashes(f.m.versionDir(tool, man.Identity))
		if err != nil {
			t.Fatal(err)
		}
		before[tool] = files
	}
	for range 5 {
		if err := f.m.Detect(ctx); err != nil {
			t.Fatal(err)
		}
		for _, tool := range toolOrder {
			f.m.cleanVersions(tool)
		}
	}
	for _, tool := range toolOrder {
		if f.m.active[tool].Identity != identities[tool] {
			t.Fatal("identity changed")
		}
		after, err := hashes(f.m.versionDir(tool, identities[tool]))
		if err != nil {
			t.Fatal(err)
		}
		if len(before[tool]) != len(after) {
			t.Fatal("files removed")
		}
		for name, sum := range before[tool] {
			if after[name] != sum {
				t.Fatal("kept current changed", name)
			}
		}
	}
}
func TestRuntimeHashChangeRemainsCorrupted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.m.InstallNeeded(ctx); err != nil {
		t.Fatal(err)
	}
	man := f.m.active[YtDlp]
	os.WriteFile(filepath.Join(f.m.versionDir(YtDlp, man.Identity), "yt-dlp.exe"), []byte("changed"), 0600)
	if err := f.m.Detect(ctx); err != nil {
		t.Fatal(err)
	}
	if f.m.statuses[YtDlp].State != "corrupted" {
		t.Fatal(f.m.statuses[YtDlp])
	}
}
