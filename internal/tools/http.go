package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const userAgent = "YTDLPGUIMangar/0.0 (managed Windows tools)"

type HTTP struct {
	Client   *http.Client
	AllowURL func(*url.URL) bool
	Create   func(string) (*os.File, error)
	Write    func(*os.File, []byte) (int, error)
}

func officialURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "raw.githubusercontent.com":
		return true
	}
	return false
}
func NewHTTP() *HTTP {
	h := &HTTP{AllowURL: officialURL, Create: func(p string) (*os.File, error) { return os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600) }}
	h.Client = &http.Client{Timeout: 20 * time.Minute, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second, IdleConnTimeout: 60 * time.Second}}
	h.Client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !h.AllowURL(req.URL) {
			return failure("redirect", "Сервер направил запрос на недопустимый адрес", "redirect rejected")
		}
		return nil
	}
	return h
}
func (h *HTTP) get(ctx context.Context, raw string) (*http.Response, error) {
	u, err := url.Parse(raw)
	if err != nil || !h.AllowURL(u) {
		return nil, failure("source", "Недопустимый источник компонента", "URL rejected")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := h.Client.Do(req)
	if err != nil { // URL errors include signed redirect queries: never retain their string.
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		message := "Не удалось подключиться к серверу. Проверьте интернет и повторите"
		if n, ok := err.(net.Error); ok && n.Timeout() {
			message = "Сервер не ответил вовремя. Повторите позже"
		}
		return nil, failure("network", message, fmt.Sprintf("host=%s transport=%T", u.Hostname(), err))
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		message := "Не удалось получить компонент. Повторите позже"
		switch resp.StatusCode {
		case 404:
			message = "Компонент отсутствует на сервере. Повторите проверку"
		case 403:
			message = "Сервер запретил запрос или исчерпан лимит GitHub. Повторите позже"
		case 429:
			message = "Сервер ограничил запросы. Повторите позже"
		}
		return nil, failure("http", message, fmt.Sprintf("host=%s status=%d", u.Hostname(), resp.StatusCode))
	}
	return resp, nil
}
func (h *HTTP) Small(ctx context.Context, raw string, limit int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	r, err := h.get(ctx, raw)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, failure("network", "Соединение прервано. Повторите", fmt.Sprintf("read=%T", err))
	}
	if int64(len(b)) > limit {
		return nil, failure("size", "Ответ сервера превышает допустимый размер", "metadata limit")
	}
	return b, nil
}
func (h *HTTP) Download(ctx context.Context, a Asset, path string, max int64, progress func(int64, int64)) (sum string, err error) {
	r, err := h.get(ctx, a.URL)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()
	if a.Size <= 0 || a.Size > max || r.ContentLength > max || (r.ContentLength >= 0 && r.ContentLength != a.Size) {
		return "", failure("size", "Размер компонента не соответствует данным выпуска", "Content-Length/asset size")
	}
	f, err := h.Create(path)
	if err != nil {
		return "", err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	hash := sha256.New()
	buf := make([]byte, 128*1024)
	var count int64
	last := time.Time{}
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		var n int
		n, err = r.Body.Read(buf)
		if n > 0 {
			count += int64(n)
			if count > max || count > a.Size {
				return "", failure("size", "Компонент превышает допустимый размер", "body size limit")
			}
			write := h.Write
			if write == nil {
				write = func(file *os.File, b []byte) (int, error) { return file.Write(b) }
			}
			if wrote, e := write(f, buf[:n]); e != nil || wrote != n {
				if e == nil {
					e = io.ErrShortWrite
				}
				return "", e
			}
			hash.Write(buf[:n])
			if progress != nil && time.Since(last) >= 150*time.Millisecond {
				progress(count, a.Size)
				last = time.Now()
			}
		}
		if err == io.EOF {
			err = nil
			break
		}
		if err != nil {
			return "", failure("network", "Скачивание прервано. Повторите", fmt.Sprintf("body read=%T", err))
		}
	}
	if count != a.Size {
		return "", failure("truncated", "Компонент скачан не полностью. Повторите", fmt.Sprintf("bytes=%d expected=%d", count, a.Size))
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if progress != nil {
		progress(count, a.Size)
	}
	sum = hex.EncodeToString(hash.Sum(nil))
	if a.Digest != "" && !strings.EqualFold(a.Digest, "sha256:"+sum) {
		return "", failure("integrity", "Проверка целостности компонента не пройдена", "SHA-256 mismatch")
	}
	return sum, nil
}
