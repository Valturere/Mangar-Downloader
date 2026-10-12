//go:build windows

package windowsplatform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/go-webview2/webviewloader"
)

func TestWebViewMissingAndCorruptFixedRuntime(t *testing.T) {
	dir := t.TempDir()
	version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(dir)
	if err != nil || version != "" {
		t.Fatalf("Отсутствующий runtime: %q, %v", version, err)
	}
	// Файл присутствует, но это не рабочий PE DLL. Проверяется detection loader.
	dllDir := filepath.Join(dir, "EBWebView", "x64")
	if err := os.MkdirAll(dllDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dllDir, "EmbeddedBrowserWebView.dll"), []byte("invalid PE"), 0600); err != nil {
		t.Fatal(err)
	}
	if version, err := webviewloader.GetAvailableCoreWebView2BrowserVersionString(dir); err == nil || version != "" {
		t.Fatalf("Повреждённый runtime принят: %q, %v", version, err)
	}
}
