//go:build windows

// Отдельный spike: не подменяет и не удаляет установленный runtime.
package main

import (
	"embed"
	"os"

	windowsplatform "YTDLP-GUI-Mangar/internal/platform/windows"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed index.html
var assets embed.FS

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	err := wails.Run(&options.App{
		Title: "Проверка WebView2", Width: 400, Height: 200,
		AssetServer: &assetserver.Options{Assets: assets},
		Windows:     &windows.Options{WebviewBrowserPath: os.Args[1], Messages: windowsplatform.WebViewMessages()},
	})
	if err != nil {
		println("Ошибка WebView2 spike:", err.Error())
		windowsplatform.ShowStartupError(windowsplatform.WebViewMessages().InvalidFixedWebview2)
		os.Exit(1)
	}
}
