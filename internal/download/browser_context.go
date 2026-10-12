package download

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CaptureTTL = 15 * time.Minute

type Cookie struct {
	Name         string        `json:"name"`
	Value        string        `json:"value"`
	Domain       string        `json:"domain"`
	Path         string        `json:"path"`
	Secure       bool          `json:"secure"`
	HTTPOnly     bool          `json:"httpOnly"`
	HostOnly     bool          `json:"hostOnly"`
	Session      bool          `json:"session"`
	Expiration   float64       `json:"expirationDate,omitempty"`
	SameSite     string        `json:"sameSite,omitempty"`
	PartitionKey *PartitionKey `json:"partitionKey,omitempty"`
}
type PartitionKey struct {
	TopLevelSite         string `json:"topLevelSite"`
	HasCrossSiteAncestor bool   `json:"hasCrossSiteAncestor,omitempty"`
}
type BrowserContext struct {
	PageTitle    string    `json:"pageTitle"`
	PageURL      string    `json:"pageURL"`
	MediaType    string    `json:"mediaType"`
	CapturedAt   time.Time `json:"capturedAt"`
	Cookies      []Cookie  `json:"cookies"`
	ExactCookies bool      `json:"exactCookies"`
}

func safeText(s string, limit int) bool {
	if len(s) > limit {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
func normalizeBrowserContext(raw string, b *BrowserContext) (*BrowserContext, error) {
	bad := errors.New("Некорректные данные браузера")
	if b == nil || !safeText(b.PageTitle, 1024) || len(b.PageURL) > 16384 || len(b.Cookies) > 64 {
		return nil, bad
	}
	if b.PageURL != "" {
		if _, err := ValidateURL(b.PageURL); err != nil {
			return nil, bad
		}
	}
	if b.MediaType != "hls" && b.MediaType != "dash" && b.MediaType != "mp4" && b.MediaType != "webm" && b.MediaType != "video" {
		return nil, bad
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, bad
	}
	out := *b
	out.Cookies = append([]Cookie(nil), b.Cookies...)
	for i, c := range out.Cookies {
		if c.Name == "" || !safeText(c.Name, 256) || strings.ContainsAny(c.Name, ";= ") || !safeText(c.Value, 8192) || !safeText(c.Domain, 253) || !safeText(c.Path, 2048) || !strings.HasPrefix(c.Path, "/") || strings.ContainsAny(c.Domain, "/\\: ") || math.IsNaN(c.Expiration) || math.IsInf(c.Expiration, 0) || c.Expiration < 0 || c.Expiration > 253402300799 {
			return nil, bad
		}
		domain := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
		if domain == "" || (c.HostOnly && !strings.EqualFold(u.Hostname(), domain)) || (!c.HostOnly && !matchesHTTPHost(u.Hostname(), domain)) || (c.Secure && u.Scheme != "https") {
			return nil, bad
		}
		p := u.EscapedPath()
		if p == "" {
			p = "/"
		}
		if p != c.Path && !(strings.HasPrefix(p, c.Path) && (strings.HasSuffix(c.Path, "/") || strings.HasPrefix(strings.TrimPrefix(p, c.Path), "/"))) {
			return nil, bad
		}
		if c.PartitionKey != nil {
			p := *c.PartitionKey
			v, e := url.Parse(p.TopLevelSite)
			if e != nil || v.Hostname() == "" || (v.Scheme != "http" && v.Scheme != "https") || v.User != nil || v.RawQuery != "" || v.Fragment != "" {
				return nil, bad
			}
			out.Cookies[i].PartitionKey = &p
		}
	}
	return &out, nil
}
func ValidateBrowserContext(raw string, b *BrowserContext) (*BrowserContext, error) {
	return normalizeBrowserContext(raw, b)
}

// ACL is set before any secret is written. Netscape loses SameSite/partition
// semantics; only the selected request's cookies are flattened, never other partitions.
func writeCookieJar(dir string, b *BrowserContext) (string, error) {
	name := filepath.Join(dir, "browser-cookies.txt")
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return "", err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return "", err
	}
	if err = windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return "", err
	}
	if _, err = fmt.Fprintln(f, "# Netscape HTTP Cookie File"); err != nil {
		return "", err
	}
	flag := func(v bool) string {
		if v {
			return "TRUE"
		}
		return "FALSE"
	}
	for _, c := range b.Cookies {
		domain := strings.TrimPrefix(c.Domain, ".")
		if !c.HostOnly {
			domain = "." + domain
		}
		if c.HTTPOnly {
			domain = "#HttpOnly_" + domain
		}
		expiry := int64(c.Expiration)
		if c.Session {
			expiry = 0
		}
		if _, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", domain, flag(!c.HostOnly), c.Path, flag(c.Secure), expiry, c.Name, c.Value); err != nil {
			return "", err
		}
	}
	return name, f.Sync()
}
func browserTitle(title string, r Request) (string, bool) {
	if sourceSecretTitle(title, r.URL) {
		title = ""
	}
	title = redactSecrets(title, r.HTTPContext, r.Browser)
	text, fallback := displayTitle(title, r.URL)
	if fallback && r.Browser != nil && strings.TrimSpace(r.Browser.PageTitle) != "" && !sensitiveName(r.Browser.PageTitle) && !sourceSecretTitle(r.Browser.PageTitle, r.URL) && !sourceSecretTitle(r.Browser.PageTitle, r.Browser.PageURL) {
		return cleanTitle(redactSecrets(strings.TrimSpace(r.Browser.PageTitle), r.HTTPContext, r.Browser)), true
	}
	return text, fallback
}

func BrowserDisplayTitle(r Request) string { title, _ := browserTitle("", r); return title }
func browserFailure(line string, r Request) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "drm") && (strings.Contains(lower, "protected") || strings.Contains(lower, "encrypted") || strings.Contains(lower, "license")) {
		return "Видео защищено DRM и не поддерживается."
	}
	msg := httpFailureMessage(line)
	if r.Browser != nil && (strings.Contains(msg, "403") || strings.Contains(msg, "401")) {
		code := "403"
		if strings.Contains(msg, "401") {
			code = "401"
		}
		msg = "Сервер отклонил запрос (HTTP " + code + "), несмотря на данные браузера."
		if time.Since(r.Browser.CapturedAt) > CaptureTTL {
			msg += " Данные браузера могли устареть. Отправьте видео из расширения снова."
		} else if len(r.Browser.Cookies) == 0 {
			msg += " Cookies для запроса отсутствуют."
		}
	}
	return msg
}
