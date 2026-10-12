package installation

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

type Metadata struct {
	Schema       int               `json:"schema"`
	ProductID    string            `json:"product_id"`
	Version      string            `json:"version"`
	Architecture string            `json:"architecture"`
	SourceCommit string            `json:"source_commit"`
	SourceDirty  bool              `json:"source_dirty,omitempty"`
	Files        map[string]string `json:"files"`
}

var PayloadNames = []string{"YTDLP-GUI-Mangar.exe", "mangar-browser-host.exe", "mangar-updater.exe"}

func ReadMetadata(dir string) (Metadata, error) {
	var m Metadata
	f, e := os.Open(filepath.Join(dir, "installation.json"))
	if e != nil {
		return m, e
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 8193))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil || m.Schema != 1 || m.ProductID != ProductID || m.Architecture != "win-x64" {
		return m, errors.New("Неизвестная установка Mangar")
	}
	if _, e = buildinfo.Parse(m.Version); e != nil {
		return m, e
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return m, errors.New("Повреждённый install manifest")
	}
	if len(m.Files) != len(PayloadNames) {
		return m, errors.New("Неверный состав установки")
	}
	for _, name := range PayloadNames {
		hash, e := hex.DecodeString(m.Files[name])
		if e != nil || len(hash) != 32 {
			return m, errors.New("Неверный состав установки")
		}
	}
	return m, nil
}
func Preflight(dir, target string) error {
	if e := SafeDirectory(dir); e != nil {
		return e
	}
	desired, e := buildinfo.Parse(target)
	if e != nil {
		return e
	}
	m, e := ReadMetadata(dir)
	if os.IsNotExist(e) {
		for _, name := range append(append([]string{}, PayloadNames...), "ru.mangar.browser.json", "uninstall.exe", ".new", ".rollback") {
			if _, e = os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(e) {
				return errors.New("Папка содержит файлы вне известной установки Mangar")
			}
		}
		return nil
	}
	if e != nil {
		return e
	}
	current, e := buildinfo.Parse(m.Version)
	if e != nil {
		return e
	}
	if desired.Compare(current) < 0 {
		return errors.New("Установка более старой версии запрещена")
	}
	return nil
}
