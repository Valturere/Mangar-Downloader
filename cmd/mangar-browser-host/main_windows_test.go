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
			if origin != hostOrigin {
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
