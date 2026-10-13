package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyYtDlpVerifiesWithoutDetectionOrMutation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.m.InstallNeeded(ctx); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(f.m.paths.Tools, string(YtDlp), "current.json")
	before, _ := os.ReadFile(current)
	// The fake exe cannot run: successful resolution also proves that no version
	// subprocess was needed for this read-only lookup.
	exe, err := ReadOnlyYtDlp(ctx, f.m.paths.Tools)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(current)
	if string(before) != string(after) {
		t.Fatal("manifest changed")
	}
	if err := os.WriteFile(exe, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOnlyYtDlp(ctx, f.m.paths.Tools); err == nil {
		t.Fatal("damaged managed exe accepted")
	}
	if _, err := ReadOnlyYtDlp(ctx, "."); err == nil {
		t.Fatal("PATH-relative root accepted")
	}
}
