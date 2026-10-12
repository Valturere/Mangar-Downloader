//go:build windows

package updates

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/installation"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"time"
)

type StageOwner struct {
	Schema    int       `json:"schema"`
	ProductID string    `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

func updatesRoot() (string, error) {
	base, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, installation.ProductID, "updates"), nil
}
func ProtectDirectory(dir string) error {
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return e
	}
	sd, e := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if e != nil {
		return e
	}
	acl, _, e := sd.DACL()
	if e != nil {
		return e
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
func NewStage(root string) (string, error) {
	if e := installation.SafeDirectory(root); e != nil {
		return "", ErrDownload
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return "", ErrDownload
	}
	if e := ProtectDirectory(root); e != nil {
		return "", ErrDownload
	}
	var id [16]byte
	if _, e := rand.Read(id[:]); e != nil {
		return "", ErrDownload
	}
	dir := filepath.Join(root, "update-"+hex.EncodeToString(id[:]))
	if e := os.Mkdir(dir, 0700); e != nil {
		return "", ErrDownload
	}
	owner, _ := json.Marshal(StageOwner{1, installation.ProductID, time.Now().UTC()})
	if e := os.WriteFile(filepath.Join(dir, "owner.json"), owner, 0600); e != nil {
		return "", ErrDownload
	}
	return dir, nil
}
func validateStage(dir, root string) error {
	if e := installation.SafeDirectory(dir); e != nil {
		return e
	}
	if filepath.Dir(dir) != root || len(filepath.Base(dir)) != 39 || filepath.Base(dir)[:7] != "update-" {
		return errors.New("Неизвестный staging обновления")
	}
	if _, e := hex.DecodeString(filepath.Base(dir)[7:]); e != nil {
		return ErrMetadata
	}
	info, e := os.Lstat(filepath.Join(dir, "owner.json"))
	if e != nil || !info.Mode().IsRegular() || info.Size() > 1024 {
		return ErrMetadata
	}
	b, e := os.ReadFile(filepath.Join(dir, "owner.json"))
	if e != nil || len(b) > 1024 {
		return ErrMetadata
	}
	var owner StageOwner
	if json.Unmarshal(b, &owner) != nil || owner.Schema != 1 || owner.ProductID != installation.ProductID {
		return ErrMetadata
	}
	return nil
}

// Bounded helper lifetimes are below 24 hours. Unknown folders or files are
// retained; cleanup never recursively removes a user directory.
func CleanupStale(root string, now time.Time) {
	if installation.SafeDirectory(root) != nil {
		return
	}
	entries, e := os.ReadDir(root)
	if e != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if validateStage(dir, root) != nil {
			continue
		}
		data, e := os.ReadFile(filepath.Join(dir, "owner.json"))
		if e != nil {
			continue
		}
		var owner StageOwner
		if json.Unmarshal(data, &owner) != nil || owner.CreatedAt.IsZero() || now.Sub(owner.CreatedAt) < 24*time.Hour {
			continue
		}
		_ = CleanupStage(dir, root)
	}
}

// Only explicit owned names are deleted. No recursive removal of user folders.
func CleanupStage(dir, root string) error {
	if e := validateStage(dir, root); e != nil {
		return e
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		name := entry.Name()
		allowed := name == "owner.json" || name == "plan.json" || name == "ready.json" || name == "result.json" || name == "mangar-updater.exe"
		if !allowed {
			stem := name
			if len(stem) > 5 && stem[len(stem)-5:] == ".part" {
				stem = stem[:len(stem)-5]
			}
			allowed = len(stem) > len("YTDLP-GUI-Mangar-Setup--win-x64.exe") && len(stem) < 100 && filepath.Ext(stem) == ".exe" && filepath.Base(stem) == stem
			if allowed {
				version := stem[len("YTDLP-GUI-Mangar-Setup-") : len(stem)-len("-win-x64.exe")]
				allowed = stem == InstallerName(version)
				if allowed {
					_, err := buildinfo.Parse(version)
					allowed = err == nil
				}
			}
		}
		if !allowed || !entry.Type().IsRegular() {
			return errors.New("Staging содержит неизвестные файлы")
		}
	}
	var failure error
	for _, entry := range entries {
		if entry.Name() == "owner.json" {
			continue
		}
		if e = os.Remove(filepath.Join(dir, entry.Name())); e != nil {
			failure = e
		}
	}
	if failure != nil {
		return failure
	}
	if e = os.Remove(filepath.Join(dir, "owner.json")); e != nil {
		return e
	}
	return os.Remove(dir)
}
