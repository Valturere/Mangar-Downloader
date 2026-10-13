package distribution

import (
	"net/url"
	"testing"
)

func TestFixedStoreIdentity(t *testing.T) {
	if ExtensionID() == "cjkbicomjehhggchaejioiehbkfoocih" || Origin() != "chrome-extension://paimennadokhgjdpgilfcebameolopbm/" {
		t.Fatal("development origin used")
	}
	u, err := url.Parse(StoreURL())
	if err != nil || u.Scheme != "https" || u.Host != "chromewebstore.google.com" || u.RawQuery != "" || u.User != nil {
		t.Fatal("unsafe store URL")
	}
}

func TestInstalledNativeOriginsAreExactlyStoreAndUnpacked(t *testing.T) {
	for _, origin := range NativeOrigins() {
		if !NativeOriginAllowed(origin) {
			t.Fatal("registered origin rejected")
		}
	}
	for _, origin := range []string{"https://youtube.com/", Origin() + "extra", UnpackedOrigin + "?query", "chrome-extension://untrusted/", ""} {
		if NativeOriginAllowed(origin) {
			t.Fatal("untrusted origin accepted")
		}
	}
	if len(NativeOrigins()) != 2 {
		t.Fatal("unexpected native authority")
	}
}
