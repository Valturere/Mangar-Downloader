package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestManualDeletionIsDetectedOnExplicitUpdateCheck(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	man := f.m.active[FFmpeg]
	os.Remove(filepath.Join(f.m.versionDir(FFmpeg, man.Identity), "ffprobe.exe"))
	if e := f.m.CheckUpdates(ctx); e != nil {
		t.Fatal(e)
	}
	if f.m.Snapshot().Tools[1].State != "corrupted" {
		t.Fatal("incomplete FFmpeg pair shown installed")
	}
	if e := f.m.InstallNeeded(ctx); e != nil {
		t.Fatal(e)
	}
	assertInstalled(t, f.m)
}
