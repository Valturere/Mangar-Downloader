//go:build windows

package process

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

// RunStreaming drains both pipes concurrently. Callbacks are serialized and must
// return promptly. An oversized line is drained, never parsed as a partial message.
// The existing Job Object runner retains ownership of the entire process tree.
func RunStreaming(ctx context.Context, executable string, args []string, dir string, line func(stream, text string)) (uint32, error) {
	return RunStreamingWithInput(ctx, executable, args, dir, nil, line)
}

// RunStreamingWithInput accepts a RAM-only native diagnostic payload.
func RunStreamingWithInput(ctx context.Context, executable string, args []string, dir string, input []byte, line func(stream, text string)) (uint32, error) {
	outR, outW, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer outR.Close()
	defer outW.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		return 0, err
	}
	defer errR.Close()
	defer errW.Close()
	var callbackMu sync.Mutex
	read := func(r io.Reader, stream string, done chan error) {
		reader := bufio.NewReaderSize(r, 64<<10)
		var b strings.Builder
		oversized := false
		for {
			part, e := reader.ReadSlice('\n')
			if !oversized && b.Len()+len(part) <= 1<<20 {
				b.Write(part)
			} else {
				oversized = true
				b.Reset()
			}
			if e != bufio.ErrBufferFull {
				if line != nil && (b.Len() > 0 || oversized) {
					value := strings.TrimRight(b.String(), "\r\n")
					if oversized {
						value = "[oversized line omitted: >1 MiB]"
					}
					callbackMu.Lock()
					line(stream, value)
					callbackMu.Unlock()
				}
				b.Reset()
				oversized = false
			}
			if e != nil && e != bufio.ErrBufferFull {
				if e == io.EOF {
					e = nil
				}
				done <- e
				return
			}
		}
	}
	outDone, errDone := make(chan error, 1), make(chan error, 1)
	go read(outR, "stdout", outDone)
	go read(errR, "stderr", errDone)
	var code uint32
	var runErr error
	if input == nil {
		code, runErr = RunInJob(ctx, executable, args, dir, outW, errW)
	} else {
		inR, inW, e := os.Pipe()
		if e != nil {
			outW.Close()
			errW.Close()
			return 0, errors.Join(e, <-outDone, <-errDone)
		}
		written := make(chan error, 1)
		go func() { _, err := io.Copy(inW, bytes.NewReader(input)); inW.Close(); written <- err }()
		code, runErr = RunInJobWithInput(ctx, executable, args, dir, inR, outW, errW)
		inR.Close()
		inW.Close()
		runErr = errors.Join(runErr, <-written)
	}
	outW.Close()
	errW.Close()
	return code, errors.Join(runErr, <-outDone, <-errDone)
}
