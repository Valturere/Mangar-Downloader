// Package apppaths задаёт единый per-user контракт путей приложения.
package apppaths

import (
	"os"
	"path/filepath"
)

type Paths struct{ Root, Tools, Data, Logs, Temp string }

func Default() (Paths, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, err
	}
	return At(filepath.Join(base, "YTDLPGUIMangar")), nil
}

func At(root string) Paths {
	return Paths{Root: root, Tools: filepath.Join(root, "tools"), Data: filepath.Join(root, "data"), Logs: filepath.Join(root, "logs"), Temp: filepath.Join(root, "temp")}
}
