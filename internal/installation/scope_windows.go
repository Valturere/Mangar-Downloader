//go:build windows

package installation

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

func KnownUserScope() (string, error) {
	return windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
}

// An installed image must share the installer gate even when a test launches it
// with isolated data. Developer images outside Programs/Mangar keep their scope.
func LaunchScope() (string, error) {
	root, e := KnownUserScope()
	if e != nil {
		return "", e
	}
	self, e := os.Executable()
	if e != nil {
		return "", e
	}
	if strings.EqualFold(filepath.Dir(self), filepath.Join(root, "Programs", "Mangar")) {
		return root, nil
	}
	return os.Getenv("LOCALAPPDATA"), nil
}
