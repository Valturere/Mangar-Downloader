package browserbridge

import (
	"YTDLP-GUI-Mangar/internal/download"
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func fixture() Message {
	return Message{Version: 1, Type: "media_capture", Capture: &Capture{URL: "https://cdn.test/video/master.m3u8?token=fixture", TabID: 1, FrameID: 2, HTTP: download.HTTPContext{Headers: map[string]string{"X-Playback": "fixture"}}, Browser: download.BrowserContext{ExactCookies: true, MediaType: "hls", PageTitle: "Тест", CapturedAt: time.Now(), Cookies: []download.Cookie{{Name: "session", Value: "secret", Domain: "cdn.test", Path: "/video", HostOnly: true, Session: true, Secure: true}}}}}
}

func TestPageURLProtocol(t *testing.T) {
	for _, source := range []string{"https://www.youtube.com/watch?v=fixture&list=fixture", "http://page.test/watch", "http://127.0.0.1/watch", "https://page.test/видео?token=PRIVATE"} {
		m := Message{Version: Version, Type: "page_url", URL: source}
		var frame bytes.Buffer
		if err := WriteFrame(&frame, m); err != nil {
			t.Fatal(err)
		}
		var got Message
		if err := ReadFrame(&frame, &got); err != nil {
			t.Fatal(err)
		}
		valid, err := Validate(got, time.Now())
		if err != nil || valid.URL != source || valid.Capture != nil {
			t.Fatal("page protocol", err)
		}
	}
	for _, source := range []string{"", "chrome://settings", "edge://newtab", "chrome-extension://id/popup.html", "file:///C:/secret", "javascript:alert(1)", "data:text/plain,test", "https://", "http://[bad", "https://page.test/%zz", "https://user:pass@page.test/", "https://page.test/\nsecret", "https://page.test/" + strings.Repeat("x", 16384)} {
		if _, err := Validate(Message{Version: Version, Type: "page_url", URL: source}, time.Now()); err == nil {
			t.Fatal("invalid page accepted")
		}
	}
	for _, m := range []Message{{Version: 2, Type: "page_url", URL: "https://page.test"}, {Version: 1, Type: "page_url", URL: "https://page.test", Capture: fixture().Capture}, {Version: 1, Type: "show", URL: "https://page.test"}, {Version: 1, Type: "media_capture", URL: "https://page.test", Capture: fixture().Capture}} {
		if _, err := Validate(m, time.Now()); err == nil {
			t.Fatal("mixed payload accepted")
		}
	}
	for _, raw := range []string{`{"version":1,"type":"page_url","url":"https://page.test","cookies":[]}`, `{"version":1,"type":"page_url","url":"https://page.test","headers":{}}`, `{"version":1,"type":"page_url","url":42}`, `{"version":1,"type":"page_url","title":"unexpected"}`} {
		var frame bytes.Buffer
		binary.Write(&frame, binary.LittleEndian, uint32(len(raw)))
		frame.WriteString(raw)
		var got Message
		if ReadFrame(&frame, &got) == nil {
			t.Fatal("unknown/invalid field accepted")
		}
	}
}

func TestPageSupportIsStrictAndHostOnly(t *testing.T) {
	m := Message{Version: Version, Type: "page_support", URL: "https://page.test/watch"}
	if _, err := ValidatePageSupport(m); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(m, time.Now()); err == nil {
		t.Fatal("support query accepted by app IPC")
	}
	for _, bad := range []Message{
		{Version: 2, Type: "page_support", URL: m.URL},
		{Version: 1, Type: "page_support", URL: "file:///SYNTHETIC_SECRET"},
		{Version: 1, Type: "page_support", URL: m.URL, Capture: fixture().Capture},
		{Version: 1, Type: "page_url", URL: m.URL},
	} {
		if _, err := ValidatePageSupport(bad); err == nil {
			t.Fatal("invalid support payload accepted")
		}
	}
}
func TestFramingAndLimits(t *testing.T) {
	m := fixture()
	var b bytes.Buffer
	if e := WriteFrame(&b, m); e != nil {
		t.Fatal(e)
	}
	var got Message
	if e := ReadFrame(&b, &got); e != nil {
		t.Fatal(e)
	}
	if got.Capture.Browser.PageTitle != "Тест" {
		t.Fatal(got)
	}
	for _, size := range []uint32{0, MaxMessage + 1, 0xffffffff} {
		var n [4]byte
		binary.LittleEndian.PutUint32(n[:], size)
		if ReadFrame(bytes.NewReader(n[:]), &got) == nil {
			t.Fatal(size)
		}
	}
	for _, raw := range []string{"{", "{} {}", `{"unknown":1}`, string([]byte{255})} {
		var frame bytes.Buffer
		binary.Write(&frame, binary.LittleEndian, uint32(len(raw)))
		frame.WriteString(raw)
		if ReadFrame(&frame, &got) == nil {
			t.Fatal("bad accepted")
		}
	}
	if WriteFrame(&b, strings.Repeat("x", MaxMessage+1)) == nil {
		t.Fatal("oversize write")
	}
}
func TestValidation(t *testing.T) {
	if _, e := Validate(fixture(), time.Now()); e != nil {
		t.Fatal(e)
	}
	mutations := []func(*Message){func(m *Message) { m.Version = 2 }, func(m *Message) { m.Type = "exec" }, func(m *Message) { m.Capture.URL = "file:///secret" }, func(m *Message) { m.Capture.URL = strings.Repeat("x", 16385) }, func(m *Message) { m.Capture.Browser.CapturedAt = time.Now().Add(-16 * time.Minute) }, func(m *Message) { m.Capture.Browser.PageTitle = "a\nb" }, func(m *Message) { m.Capture.Browser.Cookies[0].Domain = "other.test" }, func(m *Message) { m.Capture.Browser.Cookies[0].Path = "/else" }, func(m *Message) { m.Capture.Browser.Cookies[0].Value = "a\r\nb" }, func(m *Message) { m.Capture.Browser.Cookies = make([]download.Cookie, 65) }, func(m *Message) { m.Capture.AuthorizationRequired = true }, func(m *Message) {
		m.Capture.HTTP.Headers = map[string]string{}
		for i := 0; i < 33; i++ {
			m.Capture.HTTP.Headers["X-"+strings.Repeat("a", i+1)] = "v"
		}
	}}
	for i, change := range mutations {
		m := fixture()
		change(&m)
		if _, e := Validate(m, time.Now()); e == nil {
			t.Fatal("accepted", i)
		}
	}
	for _, name := range []string{"Host", "Content-Length", "Connection", "Transfer-Encoding", "Proxy-Authorization", "Upgrade", "TE", "Trailer", "Cookie", "Authorization", "X-Api-Key", "X-Auth", "Sec-WebSocket-Key"} {
		m := fixture()
		m.Capture.HTTP.Headers[name] = "s"
		if _, e := Validate(m, time.Now()); e == nil {
			t.Fatal(name)
		}
	}
}

func TestCapabilitiesAreHostOnlyAndStrict(t *testing.T) {
	valid := Message{Version: Version, Type: "get_capabilities"}
	if err := ValidateCapabilities(valid); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(valid, time.Now()); err == nil {
		t.Fatal("capabilities became an app job")
	}
	for _, m := range []Message{{Version: 2, Type: "get_capabilities"}, {Version: 1, Type: "page_url"}, {Version: 1, Type: "get_capabilities", URL: "https://secret.test"}, {Version: 1, Type: "get_capabilities", Capture: &Capture{}}} {
		if ValidateCapabilities(m) == nil {
			t.Fatal("invalid capabilities query accepted")
		}
	}
	r := Capabilities()
	if !r.OK || r.Version != 1 || r.ProtocolVersion != 2 || len(r.Capabilities) != 3 {
		t.Fatal("capabilities reply")
	}
}
