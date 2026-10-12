//go:build windows

package process

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStreamingRAMInputAndFailedStartCleanup(t *testing.T) {
	exe, _ := os.Executable()
	input := bytes.Repeat([]byte("fixture line\n"), 10000)
	count := 0
	code, e := RunStreamingWithInput(context.Background(), exe, helperArgs("stdin", t.TempDir()), "", input, func(_, s string) {
		if s != "fixture line" {
			t.Error("input changed")
		}
		count++
	})
	if e != nil || code != 0 || count != 10000 {
		t.Fatal("pipe input lost")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e = RunStreamingWithInput(ctx, exe+".missing", nil, "", input, nil); e == nil {
		t.Fatal("failed start accepted")
	}
}

func TestStreamingFloodAndCancellation(t *testing.T) {
	exe, _ := os.Executable()
	var count int
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := RunStreaming(ctx, exe, helperArgs("output", t.TempDir()), "", func(stream, line string) {
		count++
		if !strings.Contains(line, "oversized line omitted") {
			t.Error("partial line passed")
		}
	})
	if code != 0 || err != nil || count != 2 {
		t.Fatal(code, err, count)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel2()
	if _, err = RunStreaming(ctx2, exe, helperArgs("root", t.TempDir()), "", nil); err == nil {
		t.Fatal("cancel ignored")
	}
	if code, err = RunStreaming(context.Background(), exe, helperArgs("exit", t.TempDir()), "", nil); err == nil || code != 7 {
		t.Fatal(code, err)
	}
}
