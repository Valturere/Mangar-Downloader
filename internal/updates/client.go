package updates

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type asset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
	Digest string `json:"digest"`
}
type release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Assets     []asset `json:"assets"`
}
type Candidate struct {
	Manifest Manifest
	AssetID  int64
}
type apiStatusError struct {
	status int
	reason error
}

func (e *apiStatusError) Error() string { return e.reason.Error() }
func (e *apiStatusError) Unwrap() error { return e.reason }

type fileWriter interface {
	io.Writer
	Sync() error
	Close() error
}
type Client struct {
	version       string
	base          string
	http          *http.Client
	testHTTP      bool
	redirectHosts map[string]bool
	create        func(string) (fileWriter, error)
}

func NewClient(version string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = systemProxy
	transport.DisableCompression = true
	transport.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ResponseHeaderTimeout = 20 * time.Second
	return &Client{version: version, base: "https://api.github.com", http: &http.Client{Transport: transport, Timeout: 5 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, redirectHosts: map[string]bool{"release-assets.githubusercontent.com": true, "objects.githubusercontent.com": true}, create: func(p string) (fileWriter, error) { return os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) }}
}
func responseError(response *http.Response) error {
	switch response.StatusCode {
	case 401:
		return ErrAccess
	case 403:
		if response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != "" {
			return ErrRate
		}
		return ErrAccess
	case 404:
		return ErrAccess
	case 429:
		return ErrRate
	default:
		if response.StatusCode >= 500 {
			return ErrService
		}
		return ErrMetadata
	}
}
func (c *Client) request(ctx context.Context, address, accept string) (*http.Response, error) {
	u, e := url.Parse(address)
	base, e2 := url.Parse(c.base)
	if e != nil || e2 != nil || u.User != nil || u.Fragment != "" || len(address) > 16384 || (!c.testHTTP && (u.Scheme != "https" || (u.Port() != "" && u.Port() != "443"))) {
		return nil, ErrRedirect
	}
	if u.Host != base.Host && !c.redirectHosts[u.Hostname()] {
		return nil, ErrRedirect
	}
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if e != nil {
		return nil, ErrNetwork
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", "Mangar/"+c.version)
	request.Header.Set("Accept-Encoding", "identity")
	if u.Host == base.Host {
		request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	}
	response, e := c.http.Do(request)
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrNetwork
	}
	return response, nil
}
func (c *Client) apiJSON(ctx context.Context, endpoint string) ([]byte, error) {
	response, e := c.request(ctx, c.base+"/repos/"+Repository+endpoint, "application/vnd.github+json")
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &apiStatusError{response.StatusCode, responseError(response)}
	}
	b, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrNetwork
	}
	if len(b) > 1<<20 {
		return nil, ErrMetadata
	}
	return b, nil
}
func (c *Client) available(ctx context.Context) error {
	b, e := c.apiJSON(ctx, "/releases?per_page=1")
	if e != nil {
		return e
	}
	var releases []json.RawMessage
	if uniqueJSON(b) != nil || !bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) || json.Unmarshal(b, &releases) != nil || len(releases) > 1 {
		return ErrMetadata
	}
	return nil
}
func (c *Client) asset(ctx context.Context, id int64) (*http.Response, error) {
	address := c.base + "/repos/" + Repository + "/releases/assets/" + strconv.FormatInt(id, 10)
	for hop := 0; hop < 4; hop++ {
		response, e := c.request(ctx, address, "application/octet-stream")
		if e != nil {
			return nil, e
		}
		if response.StatusCode == 200 {
			return response, nil
		}
		if response.StatusCode == 302 || response.StatusCode == 301 || response.StatusCode == 303 || response.StatusCode == 307 || response.StatusCode == 308 {
			next, e := response.Location()
			response.Body.Close()
			if e != nil {
				return nil, ErrRedirect
			}
			address = next.String()
			continue
		}
		e = responseError(response)
		response.Body.Close()
		return nil, e
	}
	return nil, ErrRedirect
}
func (c *Client) Check(ctx context.Context) (*Candidate, error) {
	data, e := c.apiJSON(ctx, "/releases/latest")
	if e != nil {
		var status *apiStatusError
		if errors.As(e, &status) && status.status == 404 {
			if availableError := c.available(ctx); availableError != nil {
				return nil, availableError
			}
			return nil, ErrNoRelease
		}
		return nil, e
	}
	var r release
	if uniqueJSON(data) != nil || json.Unmarshal(data, &r) != nil || r.Draft || r.Prerelease || len(r.Assets) > 100 || !strings.HasPrefix(r.Tag, "v") {
		return nil, ErrMetadata
	}
	version := strings.TrimPrefix(r.Tag, "v")
	latest, e := buildinfo.Parse(version)
	if e != nil {
		return nil, ErrMetadata
	}
	current, e := buildinfo.Parse(c.version)
	if e != nil {
		return nil, ErrMetadata
	}
	if latest.Compare(current) <= 0 {
		return nil, nil
	}
	names := map[string]asset{}
	ids := map[int64]bool{}
	for _, a := range r.Assets {
		if a.ID <= 0 || ids[a.ID] || a.Size <= 0 || a.State != "uploaded" || names[a.Name].ID != 0 {
			return nil, ErrMetadata
		}
		names[a.Name] = a
		ids[a.ID] = true
	}
	descriptor, ok := names[ManifestName]
	if !ok || descriptor.Size > 16384 {
		return nil, ErrMetadata
	}
	installer, ok := names[InstallerName(version)]
	if !ok || installer.Size > MaxInstallerSize {
		return nil, ErrMetadata
	}
	response, e := c.asset(ctx, descriptor.ID)
	if e != nil {
		return nil, e
	}
	defer response.Body.Close()
	data, e = io.ReadAll(io.LimitReader(response.Body, 16385))
	if e != nil || int64(len(data)) != descriptor.Size {
		return nil, ErrMetadata
	}
	m, e := ParseManifest(data, version)
	if e != nil || m.InstallerSize != installer.Size {
		return nil, ErrMetadata
	}
	if installer.Digest != "" && !strings.EqualFold(installer.Digest, "sha256:"+m.InstallerSHA256) {
		return nil, ErrIntegrity
	}
	return &Candidate{Manifest: m, AssetID: installer.ID}, nil
}
func (c *Client) Download(ctx context.Context, candidate Candidate, dir string) (string, error) {
	m := candidate.Manifest
	data, _ := json.Marshal(m)
	if _, e := ParseManifest(data, m.Version); e != nil || candidate.AssetID <= 0 {
		return "", ErrMetadata
	}
	response, e := c.asset(ctx, candidate.AssetID)
	if e != nil {
		return "", e
	}
	defer response.Body.Close()
	if response.ContentLength >= 0 && response.ContentLength != m.InstallerSize {
		return "", ErrIntegrity
	}
	destination := filepath.Join(dir, m.InstallerAsset)
	if _, e = os.Lstat(destination); !os.IsNotExist(e) {
		return "", ErrDownload
	}
	partial := destination + ".part"
	file, e := c.create(partial)
	if e != nil {
		return "", ErrDownload
	}
	defer os.Remove(partial)
	sum := sha256.New()
	count, e := io.Copy(io.MultiWriter(file, sum), io.LimitReader(response.Body, m.InstallerSize+1))
	if e == nil {
		e = file.Sync()
	}
	e = errors.Join(e, file.Close())
	if e != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrDownload
	}
	expected, _ := hex.DecodeString(m.InstallerSHA256)
	if count != m.InstallerSize || !bytes.Equal(sum.Sum(nil), expected) {
		return "", ErrIntegrity
	}
	if e = os.Rename(partial, destination); e != nil {
		return "", ErrDownload
	}
	return destination, nil
}
