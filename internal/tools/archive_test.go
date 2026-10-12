package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, names []string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range names {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("fake executable/license"))
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestArchiveSafety(t *testing.T) {
	cases := []struct {
		name  string
		tool  Tool
		names []string
		want  bool
	}{
		{"Deno", Deno, []string{"deno.exe"}, true},
		{"FFmpeg", FFmpeg, []string{"build/bin/ffmpeg.exe", "build/bin/ffprobe.exe", "build/LICENSE.txt", "build/bin/ffplay.exe"}, true},
		{"traversal", Deno, []string{"../outside", "deno.exe"}, false},
		{"absolute", Deno, []string{"/outside", "deno.exe"}, false},
		{"drive", Deno, []string{"C:/outside", "deno.exe"}, false},
		{"backslash", Deno, []string{`..\outside`, "deno.exe"}, false},
		{"duplicate", Deno, []string{"deno.exe", "deno.exe"}, false},
		{"case duplicate", Deno, []string{"deno.exe", "DENO.exe"}, false},
		{"missing", Deno, []string{"unrelated"}, false},
		{"nested Deno", Deno, []string{"build/deno.exe"}, false},
		{"unexpected FFmpeg", FFmpeg, []string{"ffmpeg.exe", "ffprobe.exe", "LICENSE"}, false},
		{"duplicate executable", FFmpeg, []string{"a/bin/ffmpeg.exe", "b/bin/ffmpeg.exe", "a/bin/ffprobe.exe", "a/LICENSE"}, false},
		{"missing probe", FFmpeg, []string{"a/bin/ffmpeg.exe", "a/LICENSE"}, false},
		{"missing licenses", FFmpeg, []string{"a/bin/ffmpeg.exe", "a/bin/ffprobe.exe"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "archive.zip")
			os.WriteFile(archive, zipBytes(t, c.names), 0600)
			dest := filepath.Join(root, "dest")
			os.Mkdir(dest, 0700)
			err := extract(context.Background(), archive, dest, c.tool)
			if (err == nil) != c.want {
				t.Fatal(err)
			}
			if c.want {
				for _, exe := range executables(c.tool) {
					if _, e := os.Stat(filepath.Join(dest, exe)); e != nil {
						t.Fatal(e)
					}
				}
				if _, e := os.Stat(filepath.Join(dest, "ffplay.exe")); !os.IsNotExist(e) {
					t.Fatal("ffplay extracted")
				}
			}
		})
	}
}
func TestArchiveCorruptionLimitsAndCancellation(t *testing.T) {
	for _, mode := range []string{"corrupt", "too many", "size", "symlink", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var b bytes.Buffer
			w := zip.NewWriter(&b)
			switch mode {
			case "too many":
				for range 10001 {
					w.Create("entry" + strings.Repeat("x", 1))
				}
			case "size":
				h := zip.FileHeader{Name: "deno.exe", Method: zip.Store, UncompressedSize64: maxExtracted + 1}
				w.CreateRaw(&h)
			case "symlink":
				h := zip.FileHeader{Name: "deno.exe"}
				h.SetMode(os.ModeSymlink | 0777)
				f, _ := w.CreateHeader(&h)
				f.Write([]byte("target"))
			default:
				f, _ := w.Create("deno.exe")
				f.Write([]byte("binary"))
			}
			w.Close()
			data := b.Bytes()
			if mode == "corrupt" {
				data = []byte("invalid zip")
			}
			root := t.TempDir()
			p := filepath.Join(root, "zip")
			os.WriteFile(p, data, 0600)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			if err := extract(ctx, p, root, Deno); err == nil {
				t.Fatal("accepted failure")
			}
		})
	}
}
