//go:build windows

package download

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
	"unsafe"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"golang.org/x/sys/windows"
)

const ownerMarker = "Mangar execution temp v1\n"

var executionName = regexp.MustCompile(`^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

func tempOwner(dir string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(filepath.Join(dir, ".mangar-owner"))
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_DELETE, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), ".mangar-owner")
	if _, err = f.WriteString(ownerMarker); err == nil {
		err = f.Sync()
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// exclusiveStale proves no active process has the file open. DELETE sharing
// permits removal while this ownership handle stays held; READ/WRITE are denied.
func exclusiveStale(root, name string, before time.Time) *os.File {
	if safeTemp(root, name) != nil {
		return nil
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil
	}
	f := os.NewFile(uintptr(h), name)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !info.ModTime().Before(before) {
		f.Close()
		return nil
	}
	return f
}

// CleanupStale is startup-only. Legacy/unmarked temp, diagnostics, tools leases
// and user downloads are never touched. Errors retain files for a later startup.
func CleanupStale(paths apppaths.Paths, now time.Time) (logs, temps int) {
	jobRoot := filepath.Join(paths.Logs, "jobs")
	entries, _ := os.ReadDir(jobRoot)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" || !executionName.MatchString(entry.Name()[:len(entry.Name())-4]) {
			continue
		}
		f := exclusiveStale(jobRoot, filepath.Join(jobRoot, entry.Name()), now.Add(-30*24*time.Hour))
		if f == nil {
			continue
		}
		// FileDispositionInfo marks this exact opened file, avoiding a path replacement race.
		remove := byte(1)
		if windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo, (*byte)(unsafe.Pointer(&remove)), 1) == nil {
			logs++
		}
		f.Close()
	}
	entries, _ = os.ReadDir(paths.Temp)
	for _, entry := range entries {
		if !entry.IsDir() || !executionName.MatchString(entry.Name()) {
			continue
		}
		dir := filepath.Join(paths.Temp, entry.Name())
		f := exclusiveStale(paths.Temp, filepath.Join(dir, ".mangar-owner"), now.Add(-7*24*time.Hour))
		if f == nil {
			continue
		}
		marker, err := io.ReadAll(io.LimitReader(f, 64))
		if err == nil && string(marker) == ownerMarker && cleanupTemp(paths.Temp, dir) == nil {
			temps++
		}
		f.Close()
	}
	return logs, temps
}
