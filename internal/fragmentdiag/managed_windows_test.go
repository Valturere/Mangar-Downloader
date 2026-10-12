package fragmentdiag

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/testfixture"
)

func TestManagedSingleFragmentParity(t *testing.T) {
	if os.Getenv("MANGAR_FRAGMENT_FIXTURE") != "1" {
		t.Skip("opt-in local managed binary, no public network")
	}
	paths := testfixture.Tools(t)
	exe := testfixture.Executables(t, paths)["yt-dlp.exe"]
	for _, status := range []int{200, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var count atomic.Int32
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				if r.URL.Query().Get("token") != "QUERY_FIXTURE" || r.Header.Get("Origin") != "https://player.test" || r.Header.Get("User-Agent") != "fixture-browser" || r.Header.Get("Range") != "bytes=0-1" {
					t.Error("managed context changed")
				}
				w.Header().Set("Content-Type", "video/mp2t")
				w.WriteHeader(status)
				w.Write([]byte("media"))
			}))
			defer s.Close()
			c := fixture(s.URL + "/1.ts?token=QUERY_FIXTURE")
			c.Headers = append(c.Headers, Header{"Range", "bytes=0-1"})
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			h, err := Validate(c, time.Now())
			if err != nil {
				t.Fatal("invalid fixture")
			}
			r := Run(ctx, c)
			// Explicit local-only fixture, never the public native cycle.
			r.Ytdlp = ytdlpReplay(ctx, exe, s.URL+"/1.ts?token=QUERY_FIXTURE", h)
			r.FragmentReplays += r.Ytdlp.SentRequests
			if r.Exact.Status != status || r.Ytdlp.Status != status || count.Load() != MaxReplays || r.FragmentReplays != MaxReplays || r.Ytdlp.URL == nil || !r.Ytdlp.URL.QueryValuesEqual || !r.Ytdlp.URL.PathEqual || r.Ytdlp.Headers == nil || !r.Ytdlp.Headers.RangeEqual || !r.Ytdlp.Headers.UserAgentEqual {
				t.Fatalf("structural result: %+v count=%d", r, count.Load())
			}
		})
	}
}
