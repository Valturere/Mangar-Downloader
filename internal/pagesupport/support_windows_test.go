//go:build windows

package pagesupport

import (
	"context"
	"os"
	"testing"

	"YTDLP-GUI-Mangar/internal/apppaths"
)

func TestMatchesRequireURLUnderSpecializedExtractor(t *testing.T) {
	url := "https://page.test/watch?token=SYNTHETIC_SECRET"
	for _, tc := range []struct {
		lines            []string
		found, supported bool
	}{
		{[]string{"youtube", "vimeo", "generic", "  " + url}, true, false},
		{[]string{"generic", "  " + url, "youtube", "  " + url}, true, true},
		{[]string{"youtube", "  https://other.test", "generic", "  " + url}, true, false},
		{[]string{"generic:quoted-html", "  " + url}, true, false},
		{[]string{"UnsupportedURL", "  " + url}, true, false},
		{[]string{"vimeo", "generic"}, false, false},
	} {
		m := matches{url: url}
		for _, line := range tc.lines {
			m.line(line)
		}
		if m.found != tc.found || m.supported != tc.supported {
			t.Fatal("incorrect URL suitability classification")
		}
	}
}

func TestManagedSupportWithoutExtraction(t *testing.T) {
	if os.Getenv("MANGAR_PAGE_SUPPORT_REAL") != "1" {
		t.Skip("explicit read-only managed probe")
	}
	paths, err := apppaths.Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		url       string
		supported bool
	}{
		{"https://www.youtube.com/watch?v=BaW_jenozKc", true},
		{"https://vimeo.com/76979871", true},
		{"https://www.twitch.tv/videos/123456789", true},
		{"https://www.dailymotion.com/video/x84sh87", true},
		{"https://chatgpt.com/", false},
		{"https://unknown.example/watch", false},
	} {
		got, err := Check(context.Background(), paths.Tools, tc.url)
		if err != nil || got != tc.supported {
			t.Fatalf("read-only support mismatch (want %t): %v", tc.supported, err)
		}
	}
}
