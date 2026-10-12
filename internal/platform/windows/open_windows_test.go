//go:build windows

package windowsplatform

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenDownloadedRejectsUnknownAndMissingPaths(t *testing.T) {
	for _, path := range []string{"relative.mp4", filepath.Join(t.TempDir(), "missing.mp4"), filepath.Join(t.TempDir(), "tool.exe")} {
		for _, folder := range []bool{true, false} {
			if err := OpenDownloaded(path, folder); err == nil {
				t.Fatal("unsafe shell input", path)
			}
		}
	}
}

func TestShellRejectsRemovedSpecialPathsAndLogFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Видео & (папка) 100% 日本語")
	os.MkdirAll(dir, 0700)
	file := filepath.Join(dir, strings.Repeat("длинное имя", 10)+".mp4")
	os.WriteFile(file, []byte("fixture"), 0600)
	os.Remove(file)
	for _, folder := range []bool{false, true} {
		if err := OpenDownloaded(file, folder); err == nil || !strings.Contains(err.Error(), "не найден") {
			t.Fatal("removed file")
		}
	}
	os.Remove(dir)
	if err := OpenDownloaded(file, true); err == nil {
		t.Fatal("removed folder")
	}
	if err := OpenLogs("relative"); err == nil {
		t.Fatal("relative logs")
	}
	logFile := filepath.Join(t.TempDir(), "file")
	os.WriteFile(logFile, []byte("fixture"), 0600)
	if err := OpenLogs(logFile); err == nil {
		t.Fatal("logs is file")
	}
}

func TestNativeShellOpenSpecialPaths(t *testing.T) {
	root, source := os.Getenv("MANGAR_SHELL_SMOKE_ROOT"), os.Getenv("MANGAR_SHELL_SMOKE_FILE")
	if root == "" || source == "" {
		t.Skip("opt-in native shell; dedicated persistent smoke root and valid fixture MP4")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(source) {
		t.Fatal("absolute fixture paths required")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal("fixture MP4 unavailable")
	}
	dir := filepath.Join(root, "Видео & (папка) 100% 日本語")
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "Полный фильм & (тест) 100% 日本語 "+strings.Repeat("長", 60)+".mp4")
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = OpenDownloaded(file, true); err != nil {
		t.Fatal(err)
	}
	if err = OpenDownloaded(file, false); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(root, "logs")
	if err = OpenLogs(logs); err != nil {
		t.Fatal(err)
	}
	if stat, err := os.Stat(logs); err != nil || !stat.IsDir() {
		t.Fatal("logs directory missing")
	}
	t.Log("ShellExecuteW MP4/logs and SHOpenFolderAndSelectItems: Unicode/space/&/parentheses/%/long name; native APIs accepted; fixture retained in isolated root")
}
