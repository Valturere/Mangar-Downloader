//go:build !store

package main

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"path/filepath"
)

var hostOrigin = browserbridge.ExtensionOrigin

func acceptOrigin(origin string) bool { return origin == hostOrigin }

// Set only by the developer registration script when the helper is built in
// .local rather than beside the installed GUI. Not present in store builds.
var developmentAppPath string

func applicationPath(self string) string {
	if filepath.IsAbs(developmentAppPath) && filepath.Base(developmentAppPath) == "YTDLP-GUI-Mangar.exe" {
		return developmentAppPath
	}
	return filepath.Join(filepath.Dir(self), "YTDLP-GUI-Mangar.exe")
}
