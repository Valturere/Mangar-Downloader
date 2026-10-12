package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHTTP(server *httptest.Server) *HTTP {
	h := NewHTTP()
	h.Client.Transport = server.Client().Transport
	h.AllowURL = func(u *url.URL) bool {
		return u.Scheme == "http" && u.Host == strings.TrimPrefix(server.URL, "http://") || officialURL(u)
	}
	return h
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func TestDownloadFailures(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		body           string
		contentLength  string
		assetSize, max int64
		hash           string
		want           bool
	}{
		{"normal", 200, "test", "4", 4, 20, "sha256:" + digest([]byte("test")), true},
		{"unknown Content-Length", 200, "test", "", 4, 20, "", true},
		{"partial", 200, "xx", "", 4, 20, "", false},
		{"truncated connection", 200, "xx", "4", 4, 20, "", false},
		{"wrong length", 200, "test", "4", 5, 20, "", false},
		{"limit metadata", 200, "test", "4", 4, 3, "", false},
		{"limit stream", 200, "too-long", "", 4, 20, "", false},
		{"checksum", 200, "test", "4", 4, 20, "sha256:" + strings.Repeat("0", 64), false},
	}
	for _, code := range []int{404, 403, 429, 500, 502, 503} {
		cases = append(cases, struct {
			name           string
			status         int
			body           string
			contentLength  string
			assetSize, max int64
			hash           string
			want           bool
		}{fmt.Sprint(code), code, "", "", 4, 20, "", false})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("User-Agent") != userAgent {
					t.Error("missing UA")
				}
				if c.contentLength != "" {
					w.Header().Set("Content-Length", c.contentLength)
				}
				w.WriteHeader(c.status)
				if c.contentLength == "" {
					w.(http.Flusher).Flush()
				}
				io.WriteString(w, c.body)
			}))
			defer s.Close()
			p := filepath.Join(t.TempDir(), "artifact")
			_, err := testHTTP(s).Download(context.Background(), Asset{Size: c.assetSize, URL: s.URL, Digest: c.hash}, p, c.max, nil)
			if (err == nil) != c.want {
				t.Fatalf("err=%v", err)
			}
			if !c.want {
				if _, e := os.Stat(p); !os.IsNotExist(e) {
					t.Fatal("partial remains")
				}
			}
		})
	}
}
func TestDownloadCancellationTimeoutOffline(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(200)
		io.WriteString(w, "a")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	for _, mode := range []string{"cancel", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			h := testHTTP(s)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "timeout" {
				h.Client.Timeout = 50 * time.Millisecond
			} else {
				go func() { time.Sleep(50 * time.Millisecond); cancel() }()
			}
			p := filepath.Join(t.TempDir(), "partial")
			_, e := h.Download(ctx, Asset{Size: 100, URL: s.URL}, p, 200, nil)
			if e == nil {
				t.Fatal("no error")
			}
			if _, e = os.Stat(p); !os.IsNotExist(e) {
				t.Fatal("partial remains")
			}
		})
	}
	s.Close()
	h := testHTTP(s)
	_, err := h.Small(context.Background(), s.URL, 100)
	if err == nil {
		t.Fatal("offline succeeded")
	}
}
func TestWriteFailureAndRedirectRedaction(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://evil.invalid/file?token=secret", 302)
			return
		}
		io.WriteString(w, "test")
	}))
	defer s.Close()
	h := testHTTP(s)
	h.Create = func(string) (*os.File, error) { return nil, errors.New("injected disk full") }
	_, e := h.Download(context.Background(), Asset{Size: 4, URL: s.URL}, filepath.Join(t.TempDir(), "f"), 20, nil)
	if e == nil {
		t.Fatal("write failure lost")
	}
	h = testHTTP(s)
	_, e = h.Small(context.Background(), s.URL+"/redirect", 100)
	if e == nil || strings.Contains(e.Error(), "secret") {
		t.Fatalf("unsafe redirect: %v", e)
	}
	for _, raw := range []string{"http://github.com/a", "https://evil.invalid/a", "https://user:secret@github.com/a", "https://github.com:444/a"} {
		u, _ := url.Parse(raw)
		if officialURL(u) {
			t.Fatal(raw)
		}
	}
}
func TestChecksumParser(t *testing.T) {
	for _, b := range []string{"", strings.Repeat("x", 64) + "  a", strings.Repeat("0", 64) + "  a\n" + strings.Repeat("0", 64) + "  a"} {
		if _, e := checksumFor([]byte(b), "a"); e == nil {
			t.Fatal("accepted invalid checksum")
		}
	}
	want := strings.Repeat("a", 64)
	if got, e := checksumFor([]byte(want+" *a\n"), "a"); e != nil || got != want {
		t.Fatal(got, e)
	}
}
