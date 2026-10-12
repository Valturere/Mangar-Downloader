package updates

import (
	"YTDLP-GUI-Mangar/internal/distribution"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const fixturePrivateBody = "synthetic-private-response"

func validManifest(payload []byte) Manifest {
	sum := sha256.Sum256(payload)
	return Manifest{Schema: 1, Version: "1.0.0", Channel: "stable", Architecture: "win-x64", InstallerAsset: InstallerName("1.0.0"), InstallerSize: int64(len(payload)), InstallerSHA256: hex.EncodeToString(sum[:]), ExtensionVersion: "1.0.1", ExtensionStoreID: distribution.ExtensionID(), CreatedAt: "2026-10-12T00:00:00Z", SourceCommit: strings.Repeat("a", 40)}
}

type fixtureOptions struct {
	mutate   func(*release, *Manifest)
	status   int
	redirect string
	body     []byte
	slow     bool
}

func fixtureClient(t *testing.T, options fixtureOptions) (*Client, []byte, *atomic.Int32) {
	t.Helper()
	payload := []byte("MZ synthetic fixture installer; never executed")
	manifest := validManifest(payload)
	release := release{Tag: "v1.0.0", Assets: []asset{{ID: 1, Name: ManifestName, State: "uploaded"}, {ID: 2, Name: manifest.InstallerAsset, Size: int64(len(payload)), State: "uploaded"}}}
	if options.mutate != nil {
		options.mutate(&release, &manifest)
	}
	descriptor, _ := json.Marshal(manifest)
	release.Assets[0].Size = int64(len(descriptor))
	calls := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/repos/Valturere/Mangar-Downloader/") {
			t.Error("request escaped expected public repository")
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("anonymous request carried credentials")
			w.WriteHeader(400)
			return
		}
		if r.Header.Get("User-Agent") != "Mangar/0.9.0" || r.Header.Get("X-GitHub-Api-Version") != "2026-03-10" {
			t.Error("missing API headers")
		}
		if options.status != 0 {
			if options.status == 403 {
				w.Header().Set("X-RateLimit-Remaining", "0")
			}
			w.WriteHeader(options.status)
			_, _ = w.Write([]byte(fixturePrivateBody))
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(release)
		case strings.HasSuffix(r.URL.Path, "/assets/1"):
			_, _ = w.Write(descriptor)
		case strings.HasSuffix(r.URL.Path, "/assets/2"):
			if options.redirect != "" {
				http.Redirect(w, r, options.redirect, 302)
				return
			}
			if options.slow {
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				return
			}
			body := payload
			if options.body != nil {
				body = options.body
			}
			_, _ = w.Write(body)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient("0.9.0")
	client.base = server.URL
	client.testHTTP = true
	client.http.Transport = server.Client().Transport
	return client, payload, calls
}

func TestPublicRedirectPolicy(t *testing.T) {
	client := NewClient("0.9.0")
	for _, address := range []string{
		"http://release-assets.githubusercontent.com/asset",
		"https://untrusted.test/asset",
		"https://release-assets.githubusercontent.com:8443/asset",
		"https://user:secret@release-assets.githubusercontent.com/asset",
		"https://objects.githubusercontent.com/asset#fragment",
	} {
		if _, e := client.request(context.Background(), address, "application/octet-stream"); !errors.Is(e, ErrRedirect) {
			t.Fatal("unsafe redirect accepted", e)
		}
	}
	loop, _, calls := fixtureClient(t, fixtureOptions{redirect: "/repos/" + Repository + "/releases/assets/2"})
	if _, e := loop.asset(context.Background(), 2); !errors.Is(e, ErrRedirect) || calls.Load() != 4 {
		t.Fatal("redirect loop not bounded", e, calls.Load())
	}
}
func TestPublicReleaseDirectAndRedirectWithoutAuthentication(t *testing.T) {
	var tokenLeaked atomic.Bool
	payload := []byte("MZ synthetic fixture installer; never executed")
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			tokenLeaked.Store(true)
		}
		_, _ = w.Write(payload)
	}))
	defer storage.Close()
	for _, redirect := range []string{"", storage.URL + "/signed?fixture=1"} {
		t.Run(map[bool]string{false: "direct", true: "redirect"}[redirect != ""], func(t *testing.T) {
			client, expected, _ := fixtureClient(t, fixtureOptions{redirect: redirect})
			client.redirectHosts["127.0.0.1"] = true
			if e := client.available(context.Background()); e != nil {
				t.Fatal(e)
			}
			candidate, e := client.Check(context.Background())
			if e != nil || candidate == nil {
				t.Fatal(e)
			}
			dir := t.TempDir()
			path, e := client.Download(context.Background(), *candidate, dir)
			if e != nil {
				t.Fatal(e)
			}
			got, _ := os.ReadFile(path)
			if string(got) != string(expected) {
				t.Fatal("content mismatch")
			}
			if _, e = os.Stat(path + ".part"); !os.IsNotExist(e) {
				t.Fatal("partial remained")
			}
		})
	}
	if tokenLeaked.Load() {
		t.Fatal("storage received Authorization")
	}
}
func TestReleaseSelectionAndManifestNegativeCases(t *testing.T) {
	cases := map[string]func(*release, *Manifest){
		"draft":              func(r *release, m *Manifest) { r.Draft = true },
		"prerelease":         func(r *release, m *Manifest) { r.Prerelease = true },
		"bad-tag":            func(r *release, m *Manifest) { r.Tag = "v1.0.0-beta" },
		"wrong-architecture": func(r *release, m *Manifest) { m.Architecture = "win-arm64" },
		"wrong-schema":       func(r *release, m *Manifest) { m.Schema = 2 },
		"wrong-channel":      func(r *release, m *Manifest) { m.Channel = "nightly" },
		"version-mismatch":   func(r *release, m *Manifest) { m.Version = "1.0.1" },
		"traversal":          func(r *release, m *Manifest) { m.InstallerAsset = "../installer.exe" },
		"wrong-id":           func(r *release, m *Manifest) { m.ExtensionStoreID = strings.Repeat("a", 32) },
		"invalid-hash":       func(r *release, m *Manifest) { m.InstallerSHA256 = strings.Repeat("z", 64) },
		"wrong-size":         func(r *release, m *Manifest) { m.InstallerSize++ },
		"huge-size": func(r *release, m *Manifest) {
			m.InstallerSize = MaxInstallerSize + 1
			r.Assets[1].Size = m.InstallerSize
		},
		"missing-installer": func(r *release, m *Manifest) { r.Assets[1].Name = "wrong.exe" },
		"duplicate-name":    func(r *release, m *Manifest) { r.Assets[1].Name = ManifestName },
		"duplicate-id":      func(r *release, m *Manifest) { r.Assets[1].ID = r.Assets[0].ID },
		"digest-conflict":   func(r *release, m *Manifest) { r.Assets[1].Digest = "sha256:" + strings.Repeat("0", 64) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			client, _, _ := fixtureClient(t, fixtureOptions{mutate: mutate})
			if _, e := client.Check(context.Background()); e == nil {
				t.Fatal("invalid release accepted")
			}
		})
	}
	for _, tag := range []string{"v0.9.0", "v0.8.9"} {
		client, _, calls := fixtureClient(t, fixtureOptions{mutate: func(r *release, m *Manifest) { r.Tag = tag }})
		candidate, e := client.Check(context.Background())
		if e != nil || candidate != nil || calls.Load() != 1 {
			t.Fatal("same/older fetched assets", e)
		}
	}
}
func TestStrictManifestUnknownDuplicateTrailingAndDeepJSON(t *testing.T) {
	m := validManifest([]byte("x"))
	data, _ := json.Marshal(m)
	for _, bad := range [][]byte{
		[]byte(strings.TrimSuffix(string(data), "}") + `,"unknown":true}`),
		[]byte(strings.TrimSuffix(string(data), "}") + `,"version":"1.0.0"}`),
		append(append([]byte{}, data...), []byte(`{}`)...),
		[]byte(strings.Repeat("[", 80) + "1" + strings.Repeat("]", 80)),
		append([]byte{0xff}, data...),
	} {
		if _, e := ParseManifest(bad, "1.0.0"); e == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}
func TestSafeGitHubErrorsAndUntrustedRedirect(t *testing.T) {
	for _, status := range []int{401, 403, 404, 429, 500, 503} {
		client, _, _ := fixtureClient(t, fixtureOptions{status: status})
		_, e := client.Check(context.Background())
		if e == nil || strings.Contains(e.Error(), fixturePrivateBody) {
			t.Fatal("unsafe error")
		}
		expected := map[int]error{401: ErrAccess, 403: ErrRate, 404: ErrAccess, 429: ErrRate, 500: ErrService, 503: ErrService}[status]
		if !errors.Is(e, expected) {
			t.Fatal(status, e)
		}
	}
	client, payload, _ := fixtureClient(t, fixtureOptions{redirect: "http://untrusted.test/installer?token=fixture"})
	m := validManifest(payload)
	if _, e := client.Download(context.Background(), Candidate{m, 2}, t.TempDir()); !errors.Is(e, ErrRedirect) {
		t.Fatal(e)
	}

}

type diskFull struct{ *os.File }

func (f diskFull) Write([]byte) (int, error) { return 0, syscall.ENOSPC }
func TestPartialHashDiskFullAndCancellationNeverPublish(t *testing.T) {
	for _, name := range []string{"truncated", "hash", "disk-full", "cancel"} {
		t.Run(name, func(t *testing.T) {
			options := fixtureOptions{}
			if name == "truncated" {
				options.body = []byte("x")
			}
			if name == "hash" {
				options.body = []byte(strings.Repeat("x", len("MZ synthetic fixture installer; never executed")))
			}
			if name == "cancel" {
				options.slow = true
			}
			client, payload, _ := fixtureClient(t, options)
			if name == "disk-full" {
				client.create = func(p string) (fileWriter, error) {
					f, e := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
					return diskFull{f}, e
				}
			}
			m := validManifest(payload)
			dir := t.TempDir()
			ctx := context.Background()
			if name == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
			}
			if _, e := client.Download(ctx, Candidate{m, 2}, dir); e == nil {
				t.Fatal("bad download published")
			}
			files, e := os.ReadDir(dir)
			if e != nil || len(files) != 0 {
				t.Fatal("partial or installer remained", files, e)
			}
			if _, e = os.Stat(filepath.Join(dir, m.InstallerAsset)); !os.IsNotExist(e) {
				t.Fatal("installer exists")
			}
		})
	}
}
