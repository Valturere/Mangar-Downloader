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
