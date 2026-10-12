// Package browserbridge is the bounded local-only native protocol boundary.
package browserbridge

import (
	"YTDLP-GUI-Mangar/internal/download"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"time"
	"unicode/utf8"
)

const Version = 1
const MaxMessage = 512 * 1024
const HostName = "ru.mangar.browser"

// ExtensionOrigin is generated from the public manifest key (no private key).
const ExtensionOrigin = "chrome-extension://cjkbicomjehhggchaejioiehbkfoocih/"

type Capture struct {
	URL                   string                  `json:"url"`
	HTTP                  download.HTTPContext    `json:"http"`
	Browser               download.BrowserContext `json:"browser"`
	TabID                 int                     `json:"tabId"`
	FrameID               int                     `json:"frameId"`
	Initiator             string                  `json:"initiator,omitempty"`
	AuthorizationRequired bool                    `json:"authorizationRequired,omitempty"`
}
type Message struct {
	Version int      `json:"version"`
	Type    string   `json:"type"`
	URL     string   `json:"url,omitempty"`
	Capture *Capture `json:"capture,omitempty"`
}
type Reply struct {
	Version int    `json:"version"`
	OK      bool   `json:"ok"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Success() Reply {
	return Reply{Version: Version, OK: true, Code: "accepted", Message: "Передано в Mangar"}
}
func Failure(code string) Reply {
	text := map[string]string{"invalid": "Данные видео отклонены", "authorization": "Поток требует Authorization; этот способ пока не поддерживается", "unavailable": "Mangar не удалось запустить или он не отвечает", "closing": "Mangar завершает работу", "expired": "Данные браузера устарели. Запустите видео снова", "busy": "Слишком много ожидающих видео. Выберите папку в Mangar", "host": "Mangar не установлен или host не зарегистрирован"}[code]
	if text == "" {
		text = "Не удалось передать видео в Mangar"
	}
	return Reply{Version: Version, Code: code, Message: text}
}
func ReadFrame(r io.Reader, v any) error {
	var size [4]byte
	if _, err := io.ReadFull(r, size[:]); err != nil {
		return err
	}
	n := binary.LittleEndian.Uint32(size[:])
	if n == 0 || n > MaxMessage {
		return errors.New("Недопустимый размер сообщения")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return err
	}
	if !utf8.Valid(b) {
		return errors.New("Недопустимая кодировка")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errors.New("Некорректное сообщение")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("Лишние данные сообщения")
	}
	return nil
}
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(b) == 0 || len(b) > MaxMessage {
		return errors.New("Недопустимый размер сообщения")
	}
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	for _, part := range [][]byte{n[:], b} {
		for len(part) > 0 {
			count, e := w.Write(part)
			if e != nil {
				return e
			}
			if count == 0 {
				return io.ErrShortWrite
			}
			part = part[count:]
		}
	}
	return nil
}
func Validate(m Message, now time.Time) (Message, error) {
	bad := errors.New("Данные видео отклонены")
	if m.Version != Version {
		return Message{}, bad
	}
	if m.Type == "page_url" && m.Capture == nil {
		source, err := ValidatePageURL(m.URL)
		if err != nil {
			return Message{}, bad
		}
		m.URL = source
		return m, nil
	}
	if m.URL != "" {
		return Message{}, bad
	}
	if m.Type == "show" && m.Capture == nil {
		return m, nil
	}
	if m.Type != "media_capture" || m.Capture == nil {
		return Message{}, bad
	}
	c := *m.Capture
	if len(c.URL) > 16384 || len(c.Initiator) > 16384 || c.TabID < 0 || c.FrameID < 0 {
		return Message{}, bad
	}
	if _, err := download.ValidateURL(c.URL); err != nil {
		return Message{}, bad
	}
	if !c.Browser.CapturedAt.IsZero() && (now.Sub(c.Browser.CapturedAt) > download.CaptureTTL || c.Browser.CapturedAt.After(now.Add(time.Minute))) {
		return Message{}, bad
	}
	if c.Browser.CapturedAt.IsZero() {
		return Message{}, bad
	}
	c.HTTP.Captured = true
	h, err := download.ValidateCapturedHTTP(&c.HTTP)
	if err != nil {
		return Message{}, bad
	}
	c.HTTP = *h
	b, err := download.ValidateBrowserContext(c.URL, &c.Browser)
	if err != nil {
		return Message{}, bad
	}
	c.Browser = *b
	if !c.Browser.ExactCookies {
		return Message{}, bad
	}
	if c.AuthorizationRequired {
		return Message{}, errors.New("authorization")
	}
	m.Capture = &c
	return m, nil
}

// Page handoff follows manual-paste URL policy, including controlled local HTTP
// fixtures. No browser cookies, headers, title or media context is accepted.
func ValidatePageURL(raw string) (string, error) {
	if len(raw) > 16384 {
		return "", errors.New("Эта страница не может быть передана в Mangar")
	}
	return download.ValidateURL(raw)
}
