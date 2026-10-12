//go:build windows

package main

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/credentials"
	"YTDLP-GUI-Mangar/internal/installation"
	"YTDLP-GUI-Mangar/internal/updates"
	"fmt"
	"os"
	"strconv"
)

func main() {
	args := os.Args[1:]
	var e error
	switch {
	case len(args) == 1 && args[0] == "--version":
		fmt.Println(buildinfo.Current())
		return
	case len(args) == 1 && args[0] == "--webview":
		if !installation.WebViewAvailable() {
			os.Exit(3)
		}
		return
	case len(args) == 1 && args[0] == "--remove-credential":
		e = credentials.New("YTDLPGUIMangar/GitHubUpdates").Remove() // Legacy migration cleanup only; never reads the secret.
	case len(args) == 2 && args[0] == "--apply":
		e = updates.Apply(args[1])
	case len(args) == 3 && args[0] == "--preflight":
		e = installation.Preflight(args[1], args[2])
	case len(args) == 2 && args[0] == "--registry-preflight":
		e = installation.CheckUninstallRegistration(args[1])
	case len(args) == 3 && args[0] == "--verify-installed":
		e = installation.Verify(args[1], args[2])
	case len(args) == 2 && args[0] == "--safe-directory":
		e = installation.SafeDirectory(args[1])
		if e == nil {
			e = installation.SafeDirectory(args[1] + `\.new`)
		}
		if e == nil {
			e = installation.SafeDirectory(args[1] + `\.rollback`)
		}
	case len(args) == 2 && args[0] == "--register":
		e = installation.Register(args[1])
	case len(args) == 2 && args[0] == "--unregister":
		e = installation.Unregister(args[1])
	case len(args) == 2 && args[0] == "--remove-shortcuts":
		e = installation.RemoveShortcuts(args[1])
	case len(args) == 2 && args[0] == "--check-shortcuts":
		e = installation.CheckShortcuts(args[1])
	case len(args) == 2 && args[0] == "--check-desktop-shortcut":
		e = installation.CheckDesktopShortcut(args[1])
	case len(args) == 3 && args[0] == "--guard":
		var parent uint64
		parent, e = strconv.ParseUint(args[1], 10, 32)
		if e == nil {
			var scope string
			scope, e = installation.KnownUserScope()
			if e == nil {
				e = installation.Guard(uint32(parent), scope, args[2])
			}
		}
		if e != nil {
			_ = os.WriteFile(args[2], []byte("[guard]\r\nstatus=failed\r\n"), 0600)
		}
	default:
		os.Exit(64)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, "Не удалось выполнить операцию установки Mangar Downloader")
		os.Exit(2)
	}
}
