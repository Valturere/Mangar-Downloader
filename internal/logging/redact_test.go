package logging

import (
	"strings"
	"testing"
)

func TestRedactURLsAndMultilineCredentials(t *testing.T) {
	for _, raw := range []string{
		`https://user:SECRET@cdn.test/SIGNED_PATH?sig=SECRET#FRAGMENT`,
		"Cookie: SECRET\nAuthorization: Bearer SECRET\nX-Custom-Secret: SECRET",
		`{"token":"SECRET","url":"https://cdn.test/private?signature=SECRET"}`,
	} {
		result := Redact(raw)
		for _, private := range []string{"SECRET", "SIGNED_PATH", "FRAGMENT"} {
			if strings.Contains(result, private) {
				t.Fatal("privacy leak")
			}
		}
	}
	if got := Redact("ERROR: Connection reset by peer"); got != "ERROR: Connection reset by peer" {
		t.Fatal("tool output translated")
	}
}
