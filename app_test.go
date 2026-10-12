package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestLogsFolderUnavailableBeforeStartupOrDuringExit(t *testing.T) {
	for _, app := range []*App{{}, {ctx: context.Background(), toolClosing: true}, {ctx: context.Background(), exitPending: true}} {
		if err := app.OpenLogsFolder(); err == nil {
			t.Fatal("logs shell ran outside ready lifecycle")
		}
	}
}

func TestChoosePathSelectionCancelAndFailure(t *testing.T) {
	for _, result := range []struct {
		name, path string
		err        error
	}{
		{"unicode", `D:\Видео с пробелами & 100%\Подборка`, nil},
		{"cancel", "", nil},
		{"failure", "", errors.New("private-path?token=secret")},
	} {
		t.Run(result.name, func(t *testing.T) {
			t.Setenv("LOCALAPPDATA", t.TempDir())
			a := NewApp()
			a.startup(context.Background())
			t.Cleanup(func() { a.shutdown(context.Background()) })
			a.directoryDialog = func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
				if options.Title == "" {
					t.Fatal("Нет заголовка диалога")
				}
				return result.path, result.err
			}
			path, err := a.ChooseDirectory()
			if path != result.path || (err != nil) != (result.err != nil) {
				t.Fatalf("path=%q err=%v", path, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("В UI утекли детали ошибки")
			}
			// После cancel/error блокировка должна освобождаться.
			if !a.dialogMu.TryLock() {
				t.Fatal("Диалог остался заблокирован")
			}
			a.dialogMu.Unlock()
		})
	}
}

func TestDialogExcludesConcurrentSelection(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	a := NewApp()
	a.startup(context.Background())
	t.Cleanup(func() { a.shutdown(context.Background()) })
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	a.directoryDialog = func(context.Context, runtime.OpenDialogOptions) (string, error) {
		close(started)
		<-release
		return "", nil
	}
	go func() { _, err := a.ChooseDirectory(); done <- err }()
	<-started
	_, err := a.ChooseTXT()
	close(release)
	if err == nil {
		t.Error("Второй диалог не заблокирован")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestChooseTXTOnlySelectsTXT(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	for _, path := range []string{"", `D:\Списки & тест\файл.TXT`, `D:\video.exe`} {
		a := NewApp()
		a.startup(context.Background())
		t.Cleanup(func() { a.shutdown(context.Background()) })
		a.fileDialog = func(_ context.Context, options runtime.OpenDialogOptions) (string, error) {
			if len(options.Filters) != 1 || options.Filters[0].Pattern != "*.txt" {
				t.Fatal("Нет фильтра TXT")
			}
			return path, nil
		}
		selected, err := a.ChooseTXT()
		invalid := strings.HasSuffix(path, ".exe")
		if (err != nil) != invalid || (invalid && selected != "") || (!invalid && selected != path) {
			t.Fatalf("selected=%q err=%v", selected, err)
		}
	}
}

func TestShellBeforeStartup(t *testing.T) {
	a := NewApp()
	if _, err := a.ChooseDirectory(); err == nil {
		t.Fatal("Диалог вызван до startup")
	}
	if err := a.HideForTray(); err == nil {
		t.Fatal("Окно скрыто без tray")
	}
}
