package browserbridge

import (
	"context"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSingleInstanceIPCAndRecovery(t *testing.T) {
	id, e := UserIdentity(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	h, first, e := Claim(id)
	if e != nil || !first {
		t.Fatal(first, e)
	}
	second, first, e := Claim(id)
	if e != nil || first {
		t.Fatal(first, e)
	}
	windows.CloseHandle(second)
	var shows, captures atomic.Int32
	s, e := Listen(id, func(_ context.Context, m Message) Reply {
		if m.Type == "show" {
			shows.Add(1)
		} else {
			captures.Add(1)
		}
		return Success()
	})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if r, e := Send(ctx, id, Message{Version: 1, Type: "show"}); e != nil || !r.OK {
		t.Fatal(r, e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r, e := Send(ctx, id, fixture()); e != nil || !r.OK {
				t.Error(r, e)
			}
		}()
	}
	wg.Wait()
	if shows.Load() != 1 || captures.Load() != 8 {
		t.Fatal(shows.Load(), captures.Load())
	}
	if r, e := Send(ctx, id, Message{Version: Version, Type: "page_url", URL: "https://page.test/watch"}); e != nil || !r.OK || captures.Load() != 9 {
		t.Fatal("page pipe", r, e)
	}
	if r, e := Send(ctx, id, Message{Version: Version, Type: "page_url", URL: "file:///secret"}); e != nil || r.OK || captures.Load() != 9 {
		t.Fatal("invalid page reached handler", r, e)
	}
	// Partial frame must not keep shutdown alive.
	c, e := winio.DialPipeContext(ctx, id.Pipe)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.Write([]byte{10, 0, 0, 0})
	done := make(chan struct{})
	go func() { s.Close(); s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown stuck")
	}
	windows.CloseHandle(h)
	h, first, e = Claim(id)
	if e != nil || !first {
		t.Fatal("stale mutex", first, e)
	}
	defer windows.CloseHandle(h)
	timeout, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if _, e = AwaitSend(timeout, id, fixture()); e == nil {
		t.Fatal("missing endpoint accepted")
	}
}
