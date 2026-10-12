package main

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/installation"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// Never print raw errors/payload to either stdout or stderr.
	if len(os.Args) < 2 || os.Args[1] != hostOrigin {
		return
	}
	reply := browserbridge.Failure("invalid")
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(25 * time.Second):
			os.Exit(1)
		}
	}()
	defer close(done)
	var m browserbridge.Message
	if err := browserbridge.ReadFrame(os.Stdin, &m); err == nil {
		valid, e := browserbridge.Validate(m, time.Now())
		if e == nil {
			reply = forward(valid)
		} else if e.Error() == "authorization" {
			reply = browserbridge.Failure("authorization")
		}
	}
	browserbridge.WriteFrame(os.Stdout, reply)
}
func forward(m browserbridge.Message) browserbridge.Reply {
	guardScope, e := installation.LaunchScope()
	if e != nil || installation.Busy(guardScope) || installation.TransactionPending(filepath.Join(guardScope, "Programs", "Mangar")) {
		return browserbridge.Failure("unavailable")
	}
	id, err := browserbridge.UserIdentity(os.Getenv("LOCALAPPDATA"))
	if err != nil {
		return browserbridge.Failure("unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	r, e := browserbridge.Send(ctx, id, m)
	if e == nil {
		return r
	}
	// Only a missing endpoint starts the app. A stalled/shutting-down server is
	// allowed to respond on the next attempt; no hidden retry of a delivered capture.
	if !os.IsNotExist(e) && !isPipeMissing(e) {
		return browserbridge.Failure("unavailable")
	}
	self, err := os.Executable()
	if err != nil {
		return browserbridge.Failure("unavailable")
	}
	app := filepath.Join(filepath.Dir(self), "YTDLP-GUI-Mangar.exe")
	command := exec.Command(app)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err = command.Start(); err != nil {
		return browserbridge.Failure("unavailable")
	}
	command.Process.Release()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return browserbridge.Failure("unavailable")
		case <-ticker.C:
			r, e = browserbridge.Send(ctx, id, m)
			if e == nil {
				return r
			}
			if !os.IsNotExist(e) && !isPipeMissing(e) {
				return browserbridge.Failure("unavailable")
			}
		}
	}
}
func isPipeMissing(err error) bool { return err == syscall.ERROR_FILE_NOT_FOUND }
