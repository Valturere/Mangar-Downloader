//go:build windows

package updates

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestWaitParentFixture(t *testing.T) {
	if os.Getenv("MANGAR_WAIT_FIXTURE") != "1" {
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWaitChildFixture$")
	child.Env = os.Environ()
	if child.Start() != nil {
		os.Exit(2)
	}
	os.WriteFile(os.Getenv("MANGAR_WAIT_PID"), []byte(strconv.Itoa(child.Process.Pid)), 0600)
	_ = child.Process.Release()
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
	os.Exit(0)
}
func TestWaitChildFixture(t *testing.T) {
	if os.Getenv("MANGAR_WAIT_FIXTURE") == "1" {
		time.Sleep(600 * time.Millisecond)
		os.Exit(0)
	}
}
func TestStableParentDescendantsAndWaitCancellation(t *testing.T) {
	file := filepath.Join(t.TempDir(), "child.pid")
	command := exec.Command(os.Args[0], "-test.run=^TestWaitParentFixture$")
	command.Env = append(os.Environ(), "MANGAR_WAIT_FIXTURE=1", "MANGAR_WAIT_PID="+file)
	input, e := command.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	if command.Start() != nil {
		t.Fatal("parent start")
	}
	t.Cleanup(func() {
		input.Close()
		command.Process.Kill()
		if b, e := os.ReadFile(file); e == nil {
			if pid, e := strconv.Atoi(string(b)); e == nil {
				p, _ := os.FindProcess(pid)
				if p != nil {
					p.Kill()
				}
			}
		}
	})
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(command.Process.Pid))
	if e != nil {
		t.Fatal(e)
	}
	defer windows.CloseHandle(parent)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, e = os.Stat(file); e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e = os.Stat(file); e != nil {
		t.Fatal("child not started")
	}
	stamp, e := processStamp(parent)
	if e != nil || stamp == 0 {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	e = waitParentTree(ctx, parent, uint32(command.Process.Pid))
	cancel()
	if !errors.Is(e, ErrHelper) {
		t.Fatal("wait did not cancel", e)
	}
	state, _ := windows.WaitForSingleObject(parent, 0)
	if state != uint32(windows.WAIT_TIMEOUT) {
		t.Fatal("parent terminated by cancellation")
	}
	begin := time.Now()
	input.Close()
	command.Wait()
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if e = waitParentTree(ctx, parent, uint32(command.Process.Pid)); e != nil {
		t.Fatal(e)
	}
	if time.Since(begin) < 200*time.Millisecond {
		t.Fatal("child process outlived successful wait")
	}
}
