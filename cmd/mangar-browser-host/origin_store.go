//go:build store

package main

import (
	"YTDLP-GUI-Mangar/internal/distribution"
	"path/filepath"
)

var hostOrigin = distribution.Origin()

func acceptOrigin(origin string) bool { return distribution.NativeOriginAllowed(origin) }

func applicationPath(self string) string {
	return filepath.Join(filepath.Dir(self), "YTDLP-GUI-Mangar.exe")
}
