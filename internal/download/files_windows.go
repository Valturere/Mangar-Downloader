//go:build windows

package download

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf16"
)

func safeTemp(root, p string) error {
	if !filepath.IsAbs(root) || !filepath.IsAbs(p) {
		return errors.New("Нужен абсолютный temp path")
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return errors.New("Путь выходит за temp root")
	}
	for cur := p; ; cur = filepath.Dir(cur) {
		ptr, e := windows.UTF16PtrFromString(cur)
		if e != nil {
			return e
		}
		attr, e := windows.GetFileAttributes(ptr)
		if e == nil && attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("Temp содержит reparse point")
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return nil
}
func cleanupTemp(root, p string) error {
	if filepath.Clean(root) == filepath.Clean(p) {
		return errors.New("Нельзя удалить temp root")
	}
	if err := safeTemp(root, p); err != nil {
		return err
	}
	// Refuse junctions at any depth before deletion. Never follow external paths.
	err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return safeTemp(root, path)
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.RemoveAll(p)
}
func safeName(title, id string, limit int) string {
	clean := func(s string) string {
		return strings.TrimRight(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return -1
			}
			if strings.ContainsRune(`<>:"/\|?*`, r) {
				return '_'
			}
			return r
		}, s), " .")
	}
	if sensitiveName(title) {
		title = "Видео"
	}
	title = clean(title)
	stem := strings.ToUpper(strings.TrimRight(strings.SplitN(title, ".", 2)[0], " ."))
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || stem == "CONIN$" || stem == "CONOUT$" || (len([]rune(stem)) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && strings.ContainsRune("123456789¹²³", []rune(stem)[3])) {
		title = "_" + title
	}
	if title == "" {
		title = "Видео"
	}
	if id == "" {
		id = "video"
	}
	// IDs from generic extractors can be URL-derived or even bare tokens.
	// A stable digest is the only suffix: raw IDs never reach the filesystem.
	id = fmt.Sprintf("%x", sha256.Sum256([]byte(id)))[:8]
	value := title + " [" + id + "]"
	for len(utf16.Encode([]rune(value))) > limit && len([]rune(title)) > 1 {
		title = string([]rune(title)[:len([]rune(title))-1])
		value = title + " [" + id + "]"
	}
	// The suffix prevents exact Windows reserved device basenames (CON, COM1, etc).
	return strings.TrimRight(value, " .")
}
func publish(ctx context.Context, source, dest, title, id, jobID string, commit ...func(func() error) error) (result string, err error) {
	return publishOutput(ctx, source, dest, title, id, jobID, "", commit...)
}
func publishBatch(ctx context.Context, source, dest, filename, jobID string, commit func(func() error) error) (string, error) {
	return publishOutput(ctx, source, dest, "", "", jobID, filename, commit)
}
func publishOutput(ctx context.Context, source, dest, title, id, jobID, strictName string, commit ...func(func() error) error) (result string, err error) {
	pending := filepath.Join(dest, ".mangar-"+jobID+".tmp")
	in, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(pending, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer os.Remove(pending)
	buffer := make([]byte, 128<<10)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		var n int
		n, err = in.Read(buffer)
		if n > 0 {
			var written int
			written, err = out.Write(buffer[:n])
			if err != nil {
				break
			}
			if written != n {
				err = io.ErrShortWrite
				break
			}
		}
		if n == 0 {
			if err == io.EOF {
				err = nil
			}
			break
		}
	}
	if err == nil {
		err = out.Sync()
	}
	err = errors.Join(err, out.Close())
	if err != nil {
		return "", err
	}
	// Keep ordinary Win32 paths below 240 UTF-16 units, including collision suffix.
	budget := min(150, 240-len(utf16.Encode([]rune(dest)))-24)
	if strictName == "" && budget < 48 {
		return "", errors.New("Выбранный путь слишком длинный для безопасного имени файла")
	}
	name := safeName(title, id, budget)
	for index := 1; index <= 10000; index++ {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		suffix := ""
		if index > 1 {
			suffix = fmt.Sprintf(" (%d)", index)
		}
		candidate := filepath.Join(dest, name+suffix+".mp4")
		if strictName != "" {
			candidate = filepath.Join(dest, strictName)
		}
		a, _ := windows.UTF16PtrFromString(pending)
		b, e := windows.UTF16PtrFromString(candidate)
		if e != nil {
			return "", e
		}
		// No REPLACE_EXISTING: protects against collisions, including another app.
		rename := func() error { return windows.MoveFileEx(a, b, windows.MOVEFILE_WRITE_THROUGH) }
		if len(commit) > 0 {
			e = commit[0](rename)
		} else {
			e = rename()
		}
		if e == nil {
			return candidate, nil
		}
		if !errors.Is(e, windows.ERROR_ALREADY_EXISTS) && !errors.Is(e, windows.ERROR_FILE_EXISTS) {
			return "", e
		}
		if strictName != "" {
			return "", errors.New("Файл пакетной загрузки уже существует")
		}
	}
	return "", errors.New("Не удалось подобрать свободное имя файла")
}
