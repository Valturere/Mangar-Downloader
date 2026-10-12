package download

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func (m *Manager) admissionCookieMode() string {
	if m.cookieMode != nil {
		switch mode := m.cookieMode(); mode {
		case "auto", "chrome", "edge", "firefox", "off":
			return mode
		}
	}
	return "off" // Core clients opt in; desktop default is auto.
}

// No profile paths are accepted from the WebView or appended to argv.
func detectCookieBrowser() string {
	roots := map[string]string{"chrome": filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data"), "edge": filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data"), "firefox": filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox", "Profiles")}
	order := []string{"chrome", "edge", "firefox"}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, registry.QUERY_VALUE)
	if err == nil {
		id, _, _ := k.GetStringValue("ProgId")
		k.Close()
		id = strings.ToLower(id)
		browser := ""
		switch {
		case strings.Contains(id, "chrome"):
			browser = "chrome"
		case strings.Contains(id, "msedge"):
			browser = "edge"
		case strings.Contains(id, "firefox"):
			browser = "firefox"
		}
		if browser != "" {
			order = append([]string{browser}, order...)
		}
	}
	for _, browser := range order {
		if info, err := os.Stat(roots[browser]); err == nil && info.IsDir() {
			return browser
		}
	}
	return ""
}
func cookieFallbackEligible(line string) bool {
	s := strings.ToLower(line)
	if strings.Contains(s, "drm") || strings.Contains(s, "unsupported url") {
		return false
	}
	if match := httpErrorPattern.FindStringSubmatch(line); len(match) > 1 {
		return match[1] == "401" || match[1] == "403"
	}
	for _, marker := range []string{"http error 401", "http error 403", "status code 401", "status code 403", "login required", "login is required", "sign in", "log in", "use --cookies", "authentication required", "session expired", "cookies are required"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}
func cookieAccessFailure(line string) string {
	s := strings.ToLower(line)
	if strings.Contains(s, "could not find") && !strings.Contains(s, "cookie") && !strings.Contains(s, "local state") {
		return ""
	}
	for _, marker := range []string{"failed to decrypt", "could not copy", "unable to open database", "permission denied", "dpapi", "app-bound", "appbound", "could not find", "failed to load cookies", "database is locked", "decrypt cookie", "cookie database"} {
		if strings.Contains(s, marker) {
			return "Не удалось получить cookies браузера. Проверьте доступ к браузеру или выберите Edge, Firefox либо расширение"
		}
	}
	return ""
}

var browserPrivatePath = regexp.MustCompile(`(?i)[a-z]:[\\/][^\r\n"'<>]*?(?:cookies(?:\.sqlite)?|local state|user data|profiles)[^\r\n"'<>]*`)

func redactBrowserOutput(line string) string {
	// Extraction failures/tracebacks can reveal OS key material and profile paths.
	s := strings.ToLower(line)
	if cookieAccessFailure(line) != "" || strings.Contains(s, "traceback") || strings.Contains(s, "encrypted") || strings.Contains(s, "decrypt") || strings.HasPrefix(strings.TrimSpace(s), "file ") {
		return "[Данные ошибки доступа к cookies браузера скрыты]"
	}
	return browserPrivatePath.ReplaceAllString(redact(line), "[Путь профиля браузера скрыт]")
}
