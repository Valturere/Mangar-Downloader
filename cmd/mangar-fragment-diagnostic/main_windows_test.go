package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/fragmentdiag"
)

func TestNativeDiagnosticFramesPrivacyAndStop(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "diagnostic.exe")
	build := exec.Command("go", "build", "-o", exe, ".")
	if e := build.Run(); e != nil {
		t.Fatal("native fixture build failed")
	}
	for _, stop := range []bool{false, true} {
		var count atomic.Int32
		started := make(chan struct{}, 1)
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			started <- struct{}{}
			if stop {
				<-r.Context().Done()
				return
			}
			w.Header().Set("Content-Type", "video/mp2t")
			w.Write([]byte("media"))
		}))
		c := &fragmentdiag.Capture{Version: 1, Type: "fragment_diagnostic", CapturedAt: time.Now(), RequestID: "one", TabID: 1, FrameID: 2, Method: "GET", URL: s.URL + "/1.ts?token=SIGNED_FIXTURE", Headers: []fragmentdiag.Header{{Name: "Cookie", Value: "s=COOKIE_FIXTURE"}}, Response: fragmentdiag.Response{Status: 200}}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		command := exec.CommandContext(ctx, exe, browserbridge.ExtensionOrigin)
		in, e := command.StdinPipe()
		if e != nil {
			t.Fatal("pipe")
		}
		var out, errOut bytes.Buffer
		command.Stdout = &out
		command.Stderr = &errOut
		if e = command.Start(); e != nil {
			t.Fatal("start")
		}
		if e = browserbridge.WriteFrame(in, c); e != nil {
			t.Fatal("frame")
		}
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("no fixture request")
		}
		if stop {
			in.Close()
		}
		if e = command.Wait(); e != nil {
			t.Fatal("native fixture did not exit cleanly")
		}
		in.Close()
		cancel()
		s.Close()
		if count.Load() != 1 || errOut.Len() != 0 || strings.Contains(out.String(), "FIXTURE") {
			t.Fatal("budget or privacy")
		}
		var r fragmentdiag.Report
		if e = browserbridge.ReadFrame(&out, &r); e != nil {
			t.Fatal("report framing")
		}
		if !stop && (r.Exact.Status != 200 || !r.Exact.BodyStart) {
			t.Fatal("native result")
		}
		if stop && r.Result != "cancelled_or_expired" {
			t.Fatal("Stop ignored")
		}
	}
}
