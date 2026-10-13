// Package distribution owns the fixed production store identity; never release-provided URLs.
package distribution

import (
	_ "embed"
	"encoding/json"
	"regexp"
)

//go:embed cws.json
var source []byte
var identity = load()

func load() struct {
	ID         string `json:"extension_id"`
	PrivacyURL string `json:"privacy_url"`
} {
	var value struct {
		ID         string `json:"extension_id"`
		PrivacyURL string `json:"privacy_url"`
	}
	if json.Unmarshal(source, &value) != nil || !regexp.MustCompile(`^[a-p]{32}$`).MatchString(value.ID) {
		panic("Некорректная production CWS identity")
	}
	return value
}
func ExtensionID() string { return identity.ID }
func Origin() string      { return "chrome-extension://" + identity.ID + "/" }

// The unchanged repository manifest key identifies the explicitly loaded
// unpacked extension. Only these two fixed extension identities are trusted.
const UnpackedOrigin = "chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/"

func NativeOrigins() []string                { return []string{Origin(), UnpackedOrigin} }
func NativeOriginAllowed(origin string) bool { return origin == Origin() || origin == UnpackedOrigin }
func StoreURL() string                       { return "https://chromewebstore.google.com/detail/mangar/" + identity.ID }
func PrivacyURL() string                     { return identity.PrivacyURL }
