// Separate opt-in developer host. Never starts Mangar or admits a queue job.
package main

import (
	"context"
	"io"
	"os"
	"time"

	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/fragmentdiag"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != browserbridge.ExtensionOrigin {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go func() { <-ctx.Done(); time.Sleep(time.Second); os.Exit(1) }()
	// Chrome disconnect/Stop closes stdin. Cancel the HTTP client and Job Object.
	var c fragmentdiag.Capture
	if browserbridge.ReadFrame(os.Stdin, &c) != nil {
		browserbridge.WriteFrame(os.Stdout, fragmentdiag.Report{Result: "invalid"})
		return
	}
	defer func() { c = fragmentdiag.Capture{} }()
	go func() { io.Copy(io.Discard, os.Stdin); cancel() }()
	// Public cycle permits one Go request. Managed replay stays localhost-only
	// until child redirects can be bounded synchronously, before network sends.
	r := fragmentdiag.Run(ctx, &c)
	browserbridge.WriteFrame(os.Stdout, r)
}
