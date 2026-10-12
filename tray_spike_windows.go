//go:build windows && tray_spike

// Отдельный проверочный host Этапа 0; исключён из обычной Wails сборки.
package main

import (
	"context"
	"embed"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsoptions "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed testdata/tray/index.html
var trayTestAssets embed.FS

//go:embed build/windows/icon.ico
var trayIcon []byte

func main() {
	app := NewApp()
	if err := wails.Run(&options.App{
		Title: "Проверка tray", Width: 760, Height: 560, MinWidth: 560, MinHeight: 360, Frameless: true,
		AssetServer: &assetserver.Options{Assets: trayTestAssets},
		// Stage-0 host tests tray lifecycle only. Managed-tool startup is verified
		// separately in production and adapter tests, without external installed
		// components influencing this otherwise deterministic tray fixture.
		OnStartup: func(ctx context.Context) {
			app.mu.Lock()
			app.ctx = ctx
			app.mu.Unlock()
		}, OnDomReady: app.domReady, OnShutdown: app.shutdown, OnBeforeClose: app.beforeClose,
		Bind:    []interface{}{app},
		Windows: &windowsoptions.Options{WindowClassName: "YTDLPGUIMangarWindow"},
	}); err != nil {
		os.Exit(1)
	}
}
