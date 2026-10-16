package componentrepair

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/testfixture"
	"YTDLP-GUI-Mangar/internal/tools"
)

type SourceFunc func(context.Context, tools.Tool) (tools.Candidate, error)

func (f SourceFunc) Latest(ctx context.Context, tool tools.Tool) (tools.Candidate, error) {
	return f(ctx, tool)
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// DenoRepair serves a real, already managed executable from an isolated copy.
// Only the test Manager gets this Source/HTTP; production source policy is intact.
func DenoRepair(t *testing.T, paths apppaths.Paths) (tools.Source, *tools.HTTP, *atomic.Int32) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(paths.Tools, "deno", "current.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man tools.Manifest
	if err := json.Unmarshal(b, &man); err != nil {
		t.Fatal(err)
	}
	exe := testfixture.Executables(t, paths)["deno.exe"]
	in, err := os.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	entry, err := w.Create("deno.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(entry, in); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive.Bytes()) }))
	t.Cleanup(server.Close)
	h := tools.NewHTTP()
	h.AllowURL = func(u *url.URL) bool {
		return u.String() == server.URL+"/deno.zip" || u.Hostname() == "raw.githubusercontent.com"
	}
	base := http.DefaultTransport
	h.Client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "raw.githubusercontent.com" {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("controlled fixture notice")), Header: make(http.Header)}, nil
		}
		if r.URL.String() != server.URL+"/deno.zip" {
			return nil, fmt.Errorf("unexpected network request")
		}
		return base.RoundTrip(r)
	})
	calls := &atomic.Int32{}
	source := SourceFunc(func(ctx context.Context, tool tools.Tool) (tools.Candidate, error) {
		calls.Add(1)
		if err := ctx.Err(); err != nil {
			return tools.Candidate{}, err
		}
		if tool != tools.Deno {
			return tools.Candidate{}, fmt.Errorf("unnecessary component download: %s", tool)
		}
		asset := tools.Asset{ID: man.Asset.ID, Name: "deno.zip", Size: int64(archive.Len()), Digest: "sha256:" + hex.EncodeToString(sum[:]), URL: server.URL + "/deno.zip"}
		return tools.Candidate{Tool: tool, Repository: man.Repository, ReleaseID: man.ReleaseID, Tag: man.Tag, Asset: asset}, nil
	})
	return source, h, calls
}
