package tools

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxDownload int64 = 512 << 20
const maxExtracted uint64 = 1536 << 20

func extract(ctx context.Context, archive, dest string, t Tool) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return failure("archive", "Архив компонента повреждён", fmt.Sprintf("zip=%T", err))
	}
	defer r.Close()
	if len(r.File) > 10000 {
		return failure("archive", "В архиве слишком много файлов", "entry limit")
	}
	seen := map[string]bool{}
	expected := map[string]*zip.File{}
	licenses := map[string]*zip.File{}
	var size uint64
	ffmpegRoot := ""
	for _, f := range r.File {
		name := f.Name
		clean := path.Clean(name)
		if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") || clean == ".." || strings.HasPrefix(clean, "../") || clean != strings.TrimSuffix(name, "/") || f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return failure("archive", "Архив содержит небезопасные пути", "unsafe ZIP entry")
		}
		key := strings.ToLower(clean)
		if seen[key] {
			return failure("archive", "Архив содержит повторяющиеся файлы", "duplicate ZIP entry")
		}
		seen[key] = true
		if f.UncompressedSize64 > maxExtracted || size > maxExtracted-f.UncompressedSize64 {
			return failure("archive", "Распакованный компонент превышает допустимый размер", "extracted limit")
		}
		size += f.UncompressedSize64
		if f.FileInfo().IsDir() {
			continue
		}
		base := path.Base(name)
		for _, exe := range executables(t) {
			if strings.EqualFold(base, exe) {
				valid := name == exe
				if t == FFmpeg {
					parts := strings.Split(name, "/")
					valid = len(parts) == 3 && parts[1] == "bin" && parts[2] == exe
					if valid {
						if ffmpegRoot != "" && ffmpegRoot != parts[0] {
							valid = false
						}
						ffmpegRoot = parts[0]
					}
				}
				if !valid || expected[exe] != nil {
					return failure("archive", "Неожиданная структура архива компонента", "executable location/duplicate")
				}
				expected[exe] = f
			}
		}
		lower := strings.ToLower(base)
		if strings.HasPrefix(lower, "license") || strings.HasPrefix(lower, "copying") || strings.Contains(key, "/licenses/") {
			licenses[name] = f
		}
	}
	for _, exe := range executables(t) {
		f := expected[exe]
		if f == nil {
			return failure("archive", "В архиве отсутствует нужная программа", "missing "+exe)
		}
		if err = extractFile(ctx, f, filepath.Join(dest, exe)); err != nil {
			return err
		}
	}
	if t == FFmpeg && len(licenses) == 0 {
		return failure("license", "В архиве отсутствуют лицензии FFmpeg", "license missing")
	}
	for name, f := range licenses {
		p := filepath.Join(dest, "licenses", filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			return err
		}
		if err = extractFile(ctx, f, p); err != nil {
			return err
		}
	}
	return nil
}
func extractFile(ctx context.Context, z *zip.File, p string) (err error) {
	r, err := z.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	defer func() {
		e := f.Close()
		if err == nil {
			err = e
		}
	}()
	_, err = io.Copy(f, &contextReader{ctx: ctx, r: io.LimitReader(r, int64(maxExtracted)+1)})
	if err != nil {
		return err
	}
	return f.Sync()
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(b)
}
