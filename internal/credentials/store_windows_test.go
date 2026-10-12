//go:build windows

package credentials

import (
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
)

func TestIsolatedCredentialLifecycle(t *testing.T) {
	var id [12]byte
	_, _ = rand.Read(id[:])
	s := New(fmt.Sprintf("YTDLPGUIMangar/Test/%x", id))
	t.Cleanup(func() { _ = s.Remove() })
	if _, e := s.Read(); !errors.Is(e, ErrMissing) {
		t.Fatal("missing", e)
	}
	if e := s.Save("synthetic-fixture-secret"); e != nil {
		t.Fatal(e)
	}
	other := New(s.target)
	value, e := other.Read()
	if e != nil || value != "synthetic-fixture-secret" {
		t.Fatal("restart read failed")
	}
	if e = other.Save("replacement-fixture"); e != nil {
		t.Fatal(e)
	}
	if e = s.Remove(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(); !errors.Is(e, ErrMissing) {
		t.Fatal("remove failed")
	}
	if e = s.Remove(); e != nil {
		t.Fatal("idempotent removal")
	}
	if e = s.Save(""); e == nil {
		t.Fatal("empty accepted")
	}
}
