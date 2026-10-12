package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfficialAssetSelection(t *testing.T) {
	for _, tool := range toolOrder {
		t.Run(string(tool), func(t *testing.T) {
			name := "yt-dlp.exe"
			check := "SHA2-256SUMS"
			if tool == FFmpeg {
				name = "ffmpeg-master-latest-win64-gpl.zip"
				check = "checksums.sha256"
			}
			if tool == Deno {
				name = "deno-x86_64-pc-windows-msvc.zip"
				check = name + ".sha256sum"
			}
			prefix := "https://github.com/" + repository(tool) + "/releases/download/tag/"
			base := Release{ID: 3, Tag: "tag", Assets: []Asset{{ID: 4, Name: name, Size: 10, URL: prefix + name, Digest: "sha256:" + strings.Repeat("a", 64)}, {ID: 5, Name: check, Size: 100, URL: prefix + check}}}
			for _, mode := range []string{"valid", "prerelease", "draft", "missing", "ambiguous", "bad URL", "bad digest", "bad ID"} {
				t.Run(mode, func(t *testing.T) {
					r := base
					r.Assets = append([]Asset{}, base.Assets...)
					switch mode {
					case "prerelease":
						r.Prerelease = true
					case "draft":
						r.Draft = true
					case "missing":
						r.Assets = nil
					case "ambiguous":
						r.Assets = append(r.Assets, r.Assets[0])
					case "bad URL":
						r.Assets[0].URL = "https://evil.invalid/a"
					case "bad digest":
						r.Assets[0].Digest = "sha256:invalid"
					case "bad ID":
						r.ID = 0
					}
					s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { json.NewEncoder(w).Encode(r) }))
					defer s.Close()
					c, e := (GitHubSource{HTTP: testHTTP(s), API: s.URL}).Latest(context.Background(), tool)
					if mode == "valid" {
						if e != nil || c.Asset.Name != name || c.Checksum == nil {
							t.Fatal(c, e)
						}
					} else if e == nil {
						t.Fatal("invalid metadata accepted")
					}
				})
			}
		})
	}
}
