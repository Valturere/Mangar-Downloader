package main

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"testing"
)

func TestHostProcess(t *testing.T) {
	if os.Getenv("MANGAR_HELPER_TEST") != "1" {
		return
	}
	os.Args = []string{os.Args[0], os.Getenv("MANGAR_HELPER_ORIGIN")}
	main()
	os.Exit(0)
}
func TestHostRejectsWithoutStdoutNoiseOrSecrets(t *testing.T) {
	exe, _ := os.Executable()
	for _, origin := range []string{hostOrigin, browserbridge.ExtensionOrigin, "chrome-extension://paimennadokhgjdpgilfcebameolopbm/", "chrome-extension://untrusted/", hostOrigin + "extra"} {
		for _, payload := range []string{`{"version":2,"type":"show"}`, `{"version":1,"type":"media_capture","capture":{"url":"file:///PRIVATE_SECRET"}}`, `PRIVATE_SECRET`, `{"version":1,"type":"page_url","url":"file:///PRIVATE_SECRET"}`, `{"version":1,"type":"page_url","url":"https://page.test","cookies":["PRIVATE_SECRET"]}`} {
			var frame bytes.Buffer
			binary.Write(&frame, binary.LittleEndian, uint32(len(payload)))
			frame.WriteString(payload)
			cmd := exec.Command(exe, "-test.run=^TestHostProcess$")
			cmd.Env = append(os.Environ(), "MANGAR_HELPER_TEST=1", "MANGAR_HELPER_ORIGIN="+origin)
			cmd.Stdin = &frame
			data, e := cmd.Output()
			if e != nil {
				t.Fatal("host process")
			}
			if bytes.Contains(data, []byte("PRIVATE_SECRET")) {
				t.Fatal("secret/noise")
			}
			if !acceptOrigin(origin) {
				if len(data) != 0 {
					t.Fatal("origin accepted")
				}
				continue
			}
			var r browserbridge.Reply
			if browserbridge.ReadFrame(bytes.NewReader(data), &r) != nil || r.OK || r.Code != "invalid" {
				t.Fatal("not framed rejection")
			}
		}
	}
}

func TestHostSupportQueryDoesNotForwardOrBootApp(t *testing.T) {
	exe, _ := os.Executable()
	var frame bytes.Buffer
	if err := browserbridge.WriteFrame(&frame, browserbridge.Message{Version: 1, Type: "page_support", URL: "https://page.test/watch"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestHostProcess$")
	cmd.Env = append(os.Environ(), "MANGAR_HELPER_TEST=1", "MANGAR_HELPER_ORIGIN="+hostOrigin, "LOCALAPPDATA="+t.TempDir())
	cmd.Stdin = &frame
	data, err := cmd.Output()
	if err != nil {
		t.Fatal("host query failed")
	}
	var r browserbridge.Reply
	if browserbridge.ReadFrame(bytes.NewReader(data), &r) != nil || r.OK || r.Code != "support_unavailable" {
		t.Fatal("support query was not handled locally")
	}
}

func TestHostCapabilitiesWithoutToolsOrApplication(t *testing.T) {
	exe, _ := os.Executable()
	var input bytes.Buffer
	if err := browserbridge.WriteFrame(&input, browserbridge.Message{Version: 1, Type: "get_capabilities"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestHostProcess$")
	cmd.Env = append(os.Environ(), "MANGAR_HELPER_TEST=1", "MANGAR_HELPER_ORIGIN="+hostOrigin, "LOCALAPPDATA="+t.TempDir())
	cmd.Stdin = &input
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil || stderr.Len() != 0 {
		t.Fatal("noisy capabilities process")
	}
	var reply browserbridge.Reply
	if browserbridge.ReadFrame(bytes.NewReader(data), &reply) != nil || !reply.OK || reply.ProtocolVersion != 2 || len(reply.Capabilities) != 3 {
		t.Fatal("capabilities require app/tools")
	}
}
