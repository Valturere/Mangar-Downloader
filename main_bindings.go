//go:build bindings && !tray_spike

package main

import (
	_ "embed"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

// Wails executes a bindings-tag binary. It must not claim the production mutex,
// connect to a running instance, create runtime directories or start tools.
func main() {
	if err := wails.Run(&options.App{Bind: []interface{}{NewApp()}}); err != nil {
		panic(err)
	}
}
