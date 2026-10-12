// Package testfixture supplies isolated real-tool profiles to integration tests.
// The source profile is read-only: no Manager ever runs Detect/cleanup on it.
package testfixture

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"YTDLP-GUI-Mangar/internal/apppaths"
)

func Tools(t *testing.T) apppaths.Paths {
	t.Helper()
	source, err := apppaths.Default()
	if err != nil {
		t.Fatal(err)
	}
	if root := os.Getenv("MANGAR_TOOL_FIXTURE_ROOT"); root != "" {
		source = apppaths.At(root)
	}
	dest := apppaths.At(t.TempDir())
	for _, component := range []string{"yt-dlp", "ffmpeg", "deno"} {
		current, err := os.ReadFile(filepath.Join(source.Tools, component, "current.json"))
		if err != nil {
			t.Fatal(err)
		}
		var man struct {
			Identity string `json:"identity"`
		}
		if json.Unmarshal(current, &man) != nil || !regexp.MustCompile(`^r[1-9][0-9]*-a[1-9][0-9]*(?:-[a-f0-9]{16})?$`).MatchString(man.Identity) {
			t.Fatal("invalid fixture manifest")
		}
		// Copy only manifests and versions; never copy logs/data/temp/lock/staging.
		for _, name := range []string{"current.json", filepath.Join("versions", man.Identity)} {
			root := filepath.Join(source.Tools, component, name)
			err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(source.Tools, path)
				if err != nil {
					return err
				}
				target := filepath.Join(dest.Tools, rel)
				if entry.IsDir() {
					return os.MkdirAll(target, 0700)
				}
				if !entry.Type().IsRegular() {
					return os.ErrInvalid
				}
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					return err
				}
				in, err := os.Open(path)
				if err != nil {
					return err
				}
				defer in.Close()
				out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					return err
				}
				_, copyErr := io.Copy(out, in)
				closeErr := out.Close()
				if copyErr != nil {
					return copyErr
				}
				return closeErr
			})
			if err != nil {
				t.Fatal("Не удалось создать изолированную копию managed tools:", err)
			}
		}
	}
	return dest
}

func Executables(t *testing.T, paths apppaths.Paths) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, component := range []string{"yt-dlp", "ffmpeg", "deno"} {
		raw, err := os.ReadFile(filepath.Join(paths.Tools, component, "current.json"))
		if err != nil {
			t.Fatal(err)
		}
		var man struct {
			Identity    string            `json:"identity"`
			Executables map[string]string `json:"executables"`
		}
		if err := json.Unmarshal(raw, &man); err != nil {
			t.Fatal(err)
		}
		for name := range man.Executables {
			result[name] = filepath.Join(paths.Tools, component, "versions", man.Identity, name)
		}
	}
	return result
}
