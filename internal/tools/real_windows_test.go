//go:build windows

package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
)

// Explicit opt-in only. No videos or global toolchain installers are used.
func TestOfficialToolsSmoke(t *testing.T) {
	if os.Getenv("MANGAR_DEPENDENCY_SMOKE") != "1" {
		t.Skip("opt-in official downloads")
	}
	root := filepath.Join(t.TempDir(), "Кириллица пробелы & 100%")
	paths := apppaths.At(root)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	m := New(Options{Paths: paths, Log: ToolLogger(paths.Logs), OnChange: func(s Snapshot) {
		for _, tool := range s.Tools {
			if tool.State != "downloading" {
				continue
			}
			if tool.Bytes == tool.Total {
				t.Logf("%s downloaded %d bytes", tool.Tool, tool.Bytes)
			}
		}
	}})
	if e := m.Detect(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.CheckUpdates(ctx); e != nil {
		t.Fatal(e)
	}
	if e := m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	for _, s := range m.Snapshot().Tools {
		if s.State != "installed" {
			t.Fatalf("%+v", s)
		}
		t.Logf("%s version=%s", s.Tool, s.Version)
	}
	if p, e := m.ResolveYtDlp(ctx); e != nil {
		t.Fatal(e)
	} else {
		t.Log(p)
	}
	if p, q, e := m.ResolveFFmpegPair(ctx); e != nil {
		t.Fatal(e)
	} else {
		t.Log(p, q)
	}
	if p, e := m.ResolveDeno(ctx); e != nil {
		t.Fatal(e)
	} else {
		t.Log(p)
	}
	// A fresh manager detects installed copies locally; Source is a tripwire.
	reload := New(Options{Paths: paths, Source: sourceFunc(func(context.Context, Tool) (Candidate, error) {
		t.Fatal("startup touched network")
		return Candidate{}, nil
	})})
	if e := reload.Detect(ctx); e != nil {
		t.Fatal(e)
	}
	for _, s := range reload.Snapshot().Tools {
		if s.State != "installed" {
			t.Fatalf("reload: %+v", s)
		}
	}
	t.Log("managed smoke root:", root)
}
