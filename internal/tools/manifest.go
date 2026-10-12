package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var identityPattern = regexp.MustCompile(`^r[1-9][0-9]*-a[1-9][0-9]*(?:-[a-f0-9]{16})?$`)

func readManifest(p string, t Tool) (Manifest, error) {
	f, err := os.Open(p)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	var m Manifest
	dec := json.NewDecoder(io.LimitReader(f, 128<<10))
	if err = dec.Decode(&m); err != nil {
		return m, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return m, fmt.Errorf("Лишние данные manifest")
	}
	if m.Schema != 1 || m.Tool != t || !identityPattern.MatchString(m.Identity) || m.Repository != repository(t) || m.ReleaseID <= 0 || m.Asset.ID <= 0 || m.InstalledAt.IsZero() || len(m.Files) == 0 {
		return m, fmt.Errorf("Некорректный manifest")
	}
	for _, exe := range executables(t) {
		if m.Executables[exe] != exe || m.Versions[exe] == "" || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(m.Files[exe]) {
			return m, fmt.Errorf("Некорректный executable manifest")
		}
	}
	for name, sum := range m.Files {
		if !filepath.IsLocal(name) || filepath.Clean(name) != name || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(sum) {
			return m, fmt.Errorf("Некорректный file manifest")
		}
	}
	return m, nil
}
func writeManifest(p string, m Manifest, replace func(string, string) error) (err error) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".manifest-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(temp, p)
}
func fileHash(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func hashes(root string) (map[string]string, error) {
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if e = directorySafe(root, p); e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		sum, e := fileHash(p)
		result[rel] = sum
		return e
	})
	return result, err
}
