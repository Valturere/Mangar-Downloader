// Package updates implements anonymous public GitHub acquisition independently of Wails.
package updates

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/distribution"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"
)

const MaxInstallerSize int64 = 256 << 20
const ManifestName = "Mangar-update.json"
const Repository = "Valturere/Mangar-Downloader"

var (
	ErrAccess    = errors.New("Публичный репозиторий или релиз временно недоступен")
	ErrNoRelease = errors.New("Обновления приложения ещё не опубликованы")
	ErrRate      = errors.New("GitHub ограничил частоту запросов. Повторите позже")
	ErrService   = errors.New("GitHub временно недоступен. Повторите позже")
	ErrNetwork   = errors.New("Не удалось проверить обновления. Проверьте подключение к сети")
	ErrMetadata  = errors.New("Релиз содержит некорректные данные обновления")
	ErrIntegrity = errors.New("Размер или SHA-256 обновления не совпадает. Установщик не запущен")
	ErrDownload  = errors.New("Не удалось сохранить обновление. Проверьте сеть, свободное место и доступ к папке")
	ErrRedirect  = errors.New("Обновление перенаправлено на недопустимый адрес")
)

type Manifest struct {
	Schema           int    `json:"schema"`
	Version          string `json:"version"`
	Channel          string `json:"channel"`
	Architecture     string `json:"architecture"`
	InstallerAsset   string `json:"installer_asset"`
	InstallerSize    int64  `json:"installer_size"`
	InstallerSHA256  string `json:"installer_sha256"`
	ExtensionVersion string `json:"extension_version"`
	ExtensionStoreID string `json:"extension_store_id"`
	CreatedAt        string `json:"created_at"`
	SourceCommit     string `json:"source_commit"`
}

func InstallerName(version string) string {
	return "YTDLP-GUI-Mangar-Setup-" + version + "-win-x64.exe"
}
func uniqueJSON(data []byte) error {
	if !utf8.Valid(data) {
		return ErrMetadata
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var visit func() error
	depth := 0
	visit = func() error {
		depth++
		defer func() { depth-- }()
		if depth > 64 {
			return ErrMetadata
		}
		token, e := d.Token()
		if e != nil {
			return e
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return e
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return ErrMetadata
				}
				seen[name] = true
				if e = visit(); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return ErrMetadata
			}
		case json.Delim('['):
			for d.More() {
				if e = visit(); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return ErrMetadata
			}
		}
		return nil
	}
	if e := visit(); e != nil {
		return ErrMetadata
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrMetadata
	}
	return nil
}
func ParseManifest(data []byte, releaseVersion string) (Manifest, error) {
	var m Manifest
	if len(data) > 16384 || uniqueJSON(data) != nil {
		return m, ErrMetadata
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, ErrMetadata
	}
	if _, e := buildinfo.Parse(m.Version); e != nil {
		return m, ErrMetadata
	}
	if _, e := buildinfo.Parse(m.ExtensionVersion); e != nil {
		return m, ErrMetadata
	}
	hash, e := hex.DecodeString(m.InstallerSHA256)
	if e != nil || len(hash) != 32 {
		return m, ErrMetadata
	}
	commit, e := hex.DecodeString(m.SourceCommit)
	if e != nil || len(commit) != 20 {
		return m, ErrMetadata
	}
	if _, e = time.Parse(time.RFC3339, m.CreatedAt); e != nil {
		return m, ErrMetadata
	}
	if m.Schema != 1 || m.Channel != "stable" || m.Architecture != "win-x64" || m.Version != releaseVersion || m.InstallerAsset != InstallerName(m.Version) || m.InstallerSize <= 0 || m.InstallerSize > MaxInstallerSize || m.ExtensionStoreID != distribution.ExtensionID() {
		return m, ErrMetadata
	}
	return m, nil
}
