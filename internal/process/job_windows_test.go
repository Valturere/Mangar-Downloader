//go:build windows

package process

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// Helper выполняется отдельным процессом; никаких shell-команд.
func TestProcessHelper(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	args := os.Args[separator+1:]
	role, dir := args[0], args[1]
	if role == "stdin" {
		io.Copy(os.Stdout, os.Stdin)
		os.Exit(0)
	}
	if role == "args" {
		data, _ := json.Marshal(args[2:])
		os.Stdout.Write(data)
		os.Exit(0)
	}
	if role == "exit" {
		os.Exit(7)
	}
	if role == "media" {
		cmd := exec.Command(args[2], "--ignore-config", "--proxy", "", "--no-playlist", "-N", "8", "--downloader", "ffmpeg", "--downloader-args", "ffmpeg_i:-re", "-o", filepath.Join(dir, "result.mp4"), "--", args[3])
		cmd.Env = append(os.Environ(), "HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=", "NO_PROXY=127.0.0.1,localhost")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(10)
		}
		os.WriteFile(filepath.Join(dir, "media.pid"), []byte(strconv.Itoa(os.Getpid())), 0600)
		if err := cmd.Wait(); err != nil {
			os.Exit(11)
		}
		os.Exit(0)
	}
	if role == "output" {
		block := bytes.Repeat([]byte("x"), 64*1024)
		for range 64 {
			os.Stdout.Write(block)
			os.Stderr.Write(block)
		}
		os.Exit(0)
	}
	exe, _ := os.Executable()
	if role == "supervisor" {
		_, err := RunInJob(context.Background(), exe, helperArgs("root", dir), dir, os.Stdout, os.Stderr)
		if err != nil {
			os.Exit(9)
		}
		os.Exit(0)
	}
	if role == "root" || role == "child" || role == "orphan" {
		next := "child"
		if role == "child" || role == "orphan" {
			next = "grandchild"
		}
		cmd := exec.Command(exe, helperArgs(next, dir)...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(8)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, role+".pid"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(6)
	}
	if role == "orphan" {
		waitForPIDFile(filepath.Join(dir, "grandchild.pid"))
		os.Exit(0)
	}
	for {
		time.Sleep(time.Second)
	}
}

func helperArgs(role, dir string) []string {
	return []string{"-test.run=^TestProcessHelper$", "--", role, dir}
}

func waitForPIDFile(path string) int {
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			if pid, _ := strconv.Atoi(string(data)); pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0
}

func pidHandle(t *testing.T, dir, role string) windows.Handle {
	t.Helper()
	pid := waitForPIDFile(filepath.Join(dir, role+".pid"))
	if pid == 0 {
		t.Fatalf("Нет готового процесса %s", role)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { windows.TerminateProcess(h, 99); windows.CloseHandle(h) })
	return h
}

func assertStopped(t *testing.T, handles ...windows.Handle) {
	t.Helper()
	for _, h := range handles {
		status, err := windows.WaitForSingleObject(h, 5000)
		if err != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("Процесс остался активным: %v, status=%d", err, status)
		}
	}
}

func outputs(t *testing.T, dir string) (*os.File, *os.File) {
	t.Helper()
	out, err := os.Create(filepath.Join(dir, "stdout.txt"))
	if err != nil {
		t.Fatal(err)
	}
	errout, err := os.Create(filepath.Join(dir, "stderr.txt"))
	if err != nil {
		out.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { out.Close(); errout.Close() })
	return out, errout
}

func TestCancelTreeAndIsolateThreeJobs(t *testing.T) {
	exe, _ := os.Executable()
	type running struct {
		cancel  context.CancelFunc
		done    chan error
		handles []windows.Handle
	}
	jobs := make([]running, 3)
	for i := range jobs {
		dir := t.TempDir()
		out, errout := outputs(t, dir)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := RunInJob(ctx, exe, helperArgs("root", dir), dir, out, errout); done <- err }()
		jobs[i] = running{cancel, done, []windows.Handle{pidHandle(t, dir, "root"), pidHandle(t, dir, "child"), pidHandle(t, dir, "grandchild")}}
	}
	jobs[1].cancel()
	if err := <-jobs[1].done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	assertStopped(t, jobs[1].handles...)
	for _, i := range []int{0, 2} {
		for _, h := range jobs[i].handles {
			status, _ := windows.WaitForSingleObject(h, 0)
			if status != uint32(windows.WAIT_TIMEOUT) {
				t.Fatal("Отмена затронула соседнее дерево")
			}
		}
		jobs[i].cancel()
		if err := <-jobs[i].done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		assertStopped(t, jobs[i].handles...)
	}
}

func TestOwnerCrashKillsTree(t *testing.T) {
	exe, _ := os.Executable()
	dir := t.TempDir()
	out, errout := outputs(t, dir)
	owner := exec.Command(exe, helperArgs("supervisor", dir)...)
	owner.Stdout, owner.Stderr = out, errout
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Process.Kill(); owner.Wait() })
	handles := []windows.Handle{pidHandle(t, dir, "root"), pidHandle(t, dir, "child"), pidHandle(t, dir, "grandchild")}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	owner.Wait()
	assertStopped(t, handles...)
}

func TestNormalParentExitKillsOrphan(t *testing.T) {
	exe, _ := os.Executable()
	dir := t.TempDir()
	out, errout := outputs(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if code, err := RunInJob(ctx, exe, helperArgs("orphan", dir), dir, out, errout); err != nil || code != 0 {
		t.Fatalf("%d %v", code, err)
	}
	pid := waitForPIDFile(filepath.Join(dir, "grandchild.pid"))
	if pid == 0 {
		t.Fatal("Внук не запускался")
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	assertStopped(t, h)
}

func TestArgumentsUnicodeOutputAndFailures(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Путь с пробелами & %")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	copyExe := filepath.Join(dir, "Помощник & %.exe")
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copyExe, data, 0700); err != nil {
		t.Fatal(err)
	}
	out, errout := outputs(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"ссылка?token=НЕ_СЕКРЕТ&x=1", "100%", `цитата "a b"`, `C:\длинный путь\`, ""}
	if _, err := RunInJob(ctx, copyExe, append(helperArgs("args", dir), args...), dir, out, errout); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(out.Name())
	var decoded []string
	if err := json.Unmarshal(actual, &decoded); err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(args)
	got, _ := json.Marshal(decoded)
	if !bytes.Equal(want, got) {
		t.Fatalf("Аргументы изменены: %s", got)
	}
	if code, err := RunInJob(ctx, exe, helperArgs("exit", dir), dir, out, errout); err == nil || code != 7 {
		t.Fatalf("%d %v", code, err)
	}
	if _, err := RunInJob(ctx, exe+".missing", nil, dir, out, errout); err == nil {
		t.Fatal("Отсутствующий exe запустился")
	}
	if _, err := RunInJob(ctx, exe, []string{"nul\x00"}, dir, out, errout); err == nil {
		t.Fatal("NUL принят")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if _, err := RunInJob(cancelled, exe, nil, dir, out, errout); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := RunInJob(ctx, exe, nil, filepath.Join(dir, "нет папки"), out, errout); err == nil {
		t.Fatal("Несуществующая cwd принята")
	}
	if _, err := RunInJob(ctx, exe, helperArgs("output", dir), dir, out, errout); err != nil {
		t.Fatal(err)
	}
	for _, f := range []*os.File{out, errout} {
		stat, _ := f.Stat()
		if stat.Size() < 4*1024*1024 {
			t.Fatal("Вывод потерян")
		}
	}
	if strings.Contains(string(actual), "-test.run") {
		t.Fatal("Лишние аргументы в output")
	}
}
