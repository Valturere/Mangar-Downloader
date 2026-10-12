//go:build windows

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	mode := os.Getenv("MANGAR_TOOL_VERSION_HELPER")
	if mode != "" {
		switch mode {
		case "fail":
			fmt.Fprintln(os.Stderr, "raw fixture failure")
			os.Exit(7)
		case "flood":
			fmt.Print(strings.Repeat("a", 256<<10))
		case "hang":
			time.Sleep(time.Minute)
		case "invalid":
			fmt.Println("unexpected output")
		default:
			fmt.Println("deno 2.9.7 (stable, release, x86_64-pc-windows-msvc)")
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestExecutableVerificationRealProcessFailures(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	binary, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	for _, mode := range []string{"valid", "fail", "invalid", "flood", "hang", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "Проверка пробелы & 100%")
			os.MkdirAll(dir, 0700)
			data := binary
			if mode == "corrupt" {
				data = []byte("broken PE")
			}
			os.WriteFile(filepath.Join(dir, "deno.exe"), data, 0700)
			t.Setenv("MANGAR_TOOL_VERSION_HELPER", mode)
			// Race-instrumented helpers wait about one second before normal exit.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			versions, e := (ProcessVerifier{}).Verify(ctx, Deno, dir)
			if mode == "valid" {
				if e != nil || versions["deno.exe"] != "2.9.7" {
					t.Fatal(versions, e)
				}
			} else if e == nil {
				t.Fatal("bad executable accepted")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				e = os.Remove(filepath.Join(dir, "deno.exe"))
				if e == nil || time.Now().After(deadline) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if e != nil {
				t.Fatal("process still holds executable", e)
			}
		})
	}
}
func TestMinimumRuntimeVersion(t *testing.T) {
	for _, v := range []string{"deno 1.46.0", "deno 2.2.9", "deno bad", "deno 2.3.-1"} {
		if _, e := parseVersion(Deno, "deno.exe", v); e == nil {
			t.Fatal(v)
		}
	}
	if _, e := parseVersion(Deno, "deno.exe", "deno 2.3.0"); e != nil {
		t.Fatal(e)
	}
}
