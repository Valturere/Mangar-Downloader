//go:build !tray_spike && !bindings

package main

import (
	"context"
	"embed"
	"os"
	"path/filepath"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/installation"
	windowsplatform "YTDLP-GUI-Mangar/internal/platform/windows"
	"fmt"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(buildinfo.Current())
		return
	}
	// Create an instance of the app structure
	app := NewApp()
	guardScope, err := installation.LaunchScope()
	if err != nil {
		windowsplatform.ShowStartupError("Не удалось проверить папку установки Mangar")
		return
	}
	launchGate, err := installation.BeginLaunch(guardScope)
	if err != nil {
		windowsplatform.ShowStartupError(err.Error())
		return
	}
	id, err := browserbridge.UserIdentity(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		windows.CloseHandle(launchGate)
		windowsplatform.ShowStartupError("Не удалось подготовить локальное подключение Mangar")
		return
	}
	mutex, first, err := browserbridge.Claim(id)
	windows.CloseHandle(launchGate)
	if mutex != 0 {
		defer windows.CloseHandle(mutex)
	}
	if err != nil {
		windowsplatform.ShowStartupError("Не удалось проверить запущенный экземпляр Mangar")
		return
	}
	if !first {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if _, err = browserbridge.AwaitSend(ctx, id, browserbridge.Message{Version: 1, Type: "show"}); err != nil {
			windowsplatform.ShowStartupError("Запущенный Mangar ещё не отвечает. Повторите запуск")
		}
		return
	}
	app.browserServer, err = browserbridge.Listen(id, app.browserMessage)
	if err != nil {
		windowsplatform.ShowStartupError("Не удалось открыть локальное подключение Mangar")
		return
	}
	defer app.browserServer.Close()
	paths, err := apppaths.Default()
	if err != nil {
		println("Не удалось определить папку данных приложения:", err.Error())
		windowsplatform.ShowStartupError("Не удалось определить папку данных приложения. Проверьте LOCALAPPDATA.")
		os.Exit(1)
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title: "Mangar Downloader", Width: 760, Height: 560,
		MinWidth: 560, MinHeight: 360, Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: options.NewRGB(246, 247, 249),
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true, DisableWebViewDrop: true},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
		},
		Windows: &windowsoptions.Options{
			WebviewUserDataPath: filepath.Join(paths.Data, "webview2"),
			WindowClassName:     "YTDLPGUIMangarWindow",
			Messages:            windowsplatform.WebViewMessages(),
		},
	})

	if err != nil {
		println("Не удалось запустить приложение:", err.Error())
		// При отсутствии runtime Wails уже показал локализованный диалог.
		if err.Error() != windowsplatform.WebViewMessages().Webview2NotInstalled {
			windowsplatform.ShowStartupError("Не удалось запустить приложение. Проверьте Microsoft WebView2 Runtime и доступ к папке данных приложения.")
		}
		os.Exit(1)
	}
}
