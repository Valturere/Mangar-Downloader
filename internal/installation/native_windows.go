//go:build windows

package installation

import (
	"YTDLP-GUI-Mangar/internal/distribution"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"strings"
)

const ProductID = "YTDLPGUIMangar"
const UninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\ValturereMangar`
const developmentOrigin = "chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/"

type NativeManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}
type registryLocation struct {
	path   string
	view   uint32
	value  string
	exists bool
}

func NativePath(dir string) string { return filepath.Join(dir, "ru.mangar.browser.json") }
func nativeLocations(prefix string) []registryLocation {
	var result []registryLocation
	for _, browser := range []string{`Google\Chrome`, `Microsoft\Edge`} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			result = append(result, registryLocation{path: prefix + `\` + browser + `\NativeMessagingHosts\ru.mangar.browser`, view: view})
		}
	}
	return result
}
func readLocation(location registryLocation) (registryLocation, error) {
	k, e := registry.OpenKey(registry.CURRENT_USER, location.path, registry.QUERY_VALUE|location.view)
	if e == registry.ErrNotExist {
		return location, nil
	}
	if e != nil {
		return location, e
	}
	defer k.Close()
	location.value, _, e = k.GetStringValue("")
	if e == registry.ErrNotExist {
		return location, nil
	}
	location.exists = e == nil
	return location, e
}
func setLocation(location registryLocation, value string) error {
	k, _, e := registry.CreateKey(registry.CURRENT_USER, location.path, registry.SET_VALUE|location.view)
	if e != nil {
		return e
	}
	defer k.Close()
	return k.SetStringValue("", value)
}
func removeOwnedLocation(location registryLocation, expected string) error {
	current, e := readLocation(location)
	if e != nil {
		return e
	}
	if !current.exists || !strings.EqualFold(filepath.Clean(current.value), filepath.Clean(expected)) {
		return nil
	}
	k, e := registry.OpenKey(registry.CURRENT_USER, location.path, registry.SET_VALUE|registry.QUERY_VALUE|location.view)
	if e != nil {
		return e
	}
	e = k.DeleteValue("")
	info, statErr := k.Stat()
	k.Close()
	if e != nil {
		return e
	}
	if statErr == nil && info.ValueCount == 0 && info.SubKeyCount == 0 {
		parentPath := location.path[:strings.LastIndex(location.path, `\`)]
		parent, e := registry.OpenKey(registry.CURRENT_USER, parentPath, registry.WRITE|location.view)
		if e != nil {
			return e
		}
		defer parent.Close()
		e = registry.DeleteKey(parent, "ru.mangar.browser")
		if e != nil && e != registry.ErrNotExist {
			return e
		}
	}
	return nil
}

// The only migration accepted is the project's exact old development host.
// It does not add the development origin to the production manifest/host.
func knownDevelopmentManifest(file string) bool {
	if filepath.Base(file) != "ru.mangar.browser.json" {
		return false
	}
	b, e := os.ReadFile(file)
	if e != nil || len(b) > 8192 {
		return false
	}
	var m NativeManifest
	if json.Unmarshal(b, &m) != nil {
		return false
	}
	return m.Name == "ru.mangar.browser" && m.Description == "Mangar browser integration" && m.Type == "stdio" && filepath.Base(m.Path) == "mangar-browser-host.exe" && len(m.AllowedOrigins) == 1 && m.AllowedOrigins[0] == developmentOrigin
}
func Register(dir string) error { return registerAt(dir, `Software`) }
func registerAt(dir, prefix string) error {
	if e := SafeDirectory(dir); e != nil {
		return e
	}
	if _, e := ReadMetadata(dir); e != nil {
		return e
	}
	for _, name := range PayloadNames {
		info, e := os.Lstat(filepath.Join(dir, name))
		if e != nil || !info.Mode().IsRegular() {
			return errors.New("Неполная установка Mangar")
		}
	}
	expected := NativePath(dir)
	locations := nativeLocations(prefix)
	for i := range locations {
		location, e := readLocation(locations[i])
		if e != nil {
			return errors.New("Native Messaging registry недоступен")
		}
		if location.exists && !strings.EqualFold(filepath.Clean(location.value), expected) && !knownDevelopmentManifest(location.value) {
			return errors.New("Обнаружена чужая регистрация Native Messaging")
		}
		locations[i] = location
	}
	b, e := json.MarshalIndent(NativeManifest{"ru.mangar.browser", "Mangar Downloader", filepath.Join(dir, "mangar-browser-host.exe"), "stdio", distribution.NativeOrigins()}, "", "  ")
	if e != nil {
		return e
	}
	temporary, e := os.CreateTemp(dir, ".native-manifest-")
	if e != nil {
		return e
	}
	defer os.Remove(temporary.Name())
	_, e = temporary.Write(append(b, '\n'))
	if e == nil {
		e = temporary.Sync()
	}
	closeError := temporary.Close()
	if e == nil {
		e = closeError
	}
	if e == nil {
		e = os.Rename(temporary.Name(), expected)
	}
	if e != nil {
		return e
	}
	for i, location := range locations {
		if e = setLocation(location, expected); e != nil {
			for _, prior := range locations[:i+1] {
				if prior.exists {
					_ = setLocation(prior, prior.value)
				} else {
					_ = removeOwnedLocation(prior, expected)
				}
			}
			return errors.New("Не удалось зарегистрировать Native Messaging")
		}
	}
	return nil
}
func Unregister(dir string) error { return unregisterAt(dir, `Software`) }
func unregisterAt(dir, prefix string) error {
	if e := SafeDirectory(dir); e != nil {
		return e
	}
	for _, location := range nativeLocations(prefix) {
		if e := removeOwnedLocation(location, NativePath(dir)); e != nil {
			return e
		}
	}
	return nil
}
