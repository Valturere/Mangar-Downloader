package download

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type badTXTReader struct{}

func (badTXTReader) Read([]byte) (int, error) { return 0, errors.New("secret read failure") }
func txtFixture(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Список & 100% 日本語.txt")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestTXTParsing(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		for _, bom := range []string{"", "\ufeff"} {
			input := bom + "  https://host/видео?a=100%25  " + newline + "   " + newline + "invalid text" + newline + "https://host/видео?a=100%25"
			entries, err := ParseTXT(strings.NewReader(input))
			if err != nil || len(entries) != 3 {
				t.Fatal(entries, err)
			}
			if entries[0].Index != 1 || entries[1].Index != 2 || !entries[1].Invalid || entries[2].Index != 3 || entries[0].URL != entries[2].URL {
				t.Fatal(entries)
			}
		}
	}
	for _, n := range []int{1, 100, 300, 1000} {
		entries, err := ParseTXT(strings.NewReader(strings.Repeat("https://host/a\n", n)))
		if err != nil || len(entries) != n || entries[n-1].Index != n {
			t.Fatal(n, err)
		}
	}
	for _, input := range []string{"\n  \n", string([]byte{255, 254}), "https://host/\x00", strings.Repeat("a", MaxTXTLineBytes+1), strings.Repeat("https://host/a\n", 1001), strings.Repeat(" \n", MaxTXTBytes/2+1)} {
		if _, err := ParseTXT(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid file length=%d", len(input))
		}
	}
	if _, err := ParseTXT(io.MultiReader(strings.NewReader("https://host/a\n"), badTXTReader{})); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
}
func TestTXTFileErrors(t *testing.T) {
	path := txtFixture(t, "https://host/a")
	if _, err := readTXT(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := readTXT(path); err == nil {
		t.Fatal("missing accepted")
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := readTXT(path); err == nil {
		t.Fatal("directory accepted")
	}
	large := txtFixture(t, strings.Repeat(" ", MaxTXTBytes+1))
	if _, err := readTXT(large); err == nil {
		t.Fatal("large accepted")
	}
	if _, err := readTXT("relative.txt"); err == nil {
		t.Fatal("relative accepted")
	}
	locked := txtFixture(t, "https://host/a")
	ptr, err := windows.UTF16PtrFromString(locked)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if _, err := readTXT(locked); err == nil {
		t.Fatal("exclusive/read failure accepted")
	}
}

func TestRetryFromTerminalCallbackKeepsAttemptResourcesSeparate(t *testing.T) {
	m, dest, _ := fixtureManager(t, "exit")
	firstDone := make(chan struct{})
	retryAdmitted := make(chan struct{})
	var runs atomic.Int32
	var once sync.Once
	m.callback = func(s Snapshot) {
		if len(s.Jobs) == 0 || s.Jobs[0].State != "failed" {
			return
		}
		once.Do(func() {
			m.mu.Lock()
			firstDone = m.jobs[0].done
			m.mu.Unlock()
			if _, err := m.Retry(s.Jobs[0].ID); err != nil {
				t.Error(err)
			}
			close(retryAdmitted)
		})
	}
	// Once.Do is non-reentrant: the retry runner waits until the first callback returns.
	base := m.run
	m.run = func(ctx context.Context, exe string, args []string, dir string, line func(string, string)) (uint32, error) {
		if runs.Add(1) > 1 {
			select {
			case <-retryAdmitted:
			case <-ctx.Done():
				return 1, ctx.Err()
			}
			<-ctx.Done()
			return 1, ctx.Err()
		}
		return base(ctx, exe, args, dir, line)
	}
	if _, err := m.Start(Request{URL: "https://host/a", Directory: dest}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-retryAdmitted:
	case <-time.After(5 * time.Second):
		t.Fatal("retry callback stuck")
	}
	select {
	case <-firstDone:
	case <-time.After(5 * time.Second):
		t.Fatal("old attempt closed replacement channel")
	}
	m.mu.Lock()
	retryDone := m.jobs[0].done
	m.mu.Unlock()
	select {
	case <-retryDone:
		t.Fatal("retry completion channel closed by old attempt")
	default:
	}
}
func TestBatchInvalidNoResources(t *testing.T) {
	m, dest, releases := fixtureManager(t, "")
	result, err := m.ImportTXT(txtFixture(t, "invalid\n--exec evil\nfile:///C:/tool.exe"), dest)
	if err != nil || result.Invalid != 3 || result.Queued != 0 || result.Snapshot.Active != 0 {
		t.Fatal(result, err)
	}
	for _, job := range result.Snapshot.Jobs {
		if !job.InvalidSource || job.State != "failed" {
			t.Fatal(job)
		}
		if _, err := m.Retry(job.ID); err == nil {
			t.Fatal("invalid retried")
		}
	}
	if releases.Load() != 0 {
		t.Fatal("invalid acquired leases")
	}
	for _, dir := range []string{m.paths.Temp, m.paths.Logs} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("invalid created resources", dir, err)
		}
	}
}
func TestBatchPreflightAndReservations(t *testing.T) {
	for _, indices := range [][]int{{1}, {17}, {1, 8, 17, 18, 19, 20}} {
		t.Run(fmt.Sprint(indices), func(t *testing.T) {
			m, dest, _ := fixtureManager(t, "blocked")
			for _, i := range indices {
				if err := os.WriteFile(filepath.Join(dest, batchName(i)), []byte("external"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := m.ImportTXT(txtFixture(t, strings.Repeat("https://host/a\n", 20)), dest)
			if err == nil || !strings.Contains(err.Error(), batchName(indices[0])) || len(m.Snapshot().Jobs) != 0 || m.Snapshot().Active != 0 {
				t.Fatal(err, m.Snapshot())
			}
		})
	}
	m, dest, _ := fixtureManager(t, "blocked")
	path := txtFixture(t, strings.Repeat("https://host/a\n", 20))
	first, err := m.ImportTXT(path, dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.ImportTXT(path, dest); err == nil || len(m.Snapshot().Jobs) != 20 {
		t.Fatal("reservation failed", err)
	}
	other := t.TempDir()
	second, err := m.ImportTXT(path, other)
	if err != nil || first.BatchID == second.BatchID || len(m.Snapshot().Jobs) != 40 {
		t.Fatal(err)
	}
	// Invalid position has no output reservation and an existing file there is allowed.
	third := t.TempDir()
	os.WriteFile(filepath.Join(third, "1.mp4"), []byte("external"), 0600)
	if _, err := m.ImportTXT(txtFixture(t, "invalid\nhttps://host/a"), third); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentBatchAtomicAdmission(t *testing.T) {
	m, dest, _ := fixtureManager(t, "blocked")
	path := txtFixture(t, strings.Repeat("https://host/a\n", 20))
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Go(func() { _, err := m.ImportTXT(path, dest); results <- err })
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		}
	}
	if accepted != 1 || len(m.Snapshot().Jobs) != 20 || m.Snapshot().Active != 3 {
		t.Fatal(accepted, m.Snapshot())
	}
}
func TestBatchStableNumbersFIFOAndStress(t *testing.T) {
	for _, n := range []int{20, 100, 300} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			m, dest, starts, _ := controlledManager(t)
			path := txtFixture(t, strings.Repeat("https://host/a\n", n))
			result, err := m.ImportTXT(path, dest)
			if err != nil {
				t.Fatal(err)
			}
			jobs := result.Snapshot.Jobs
			if len(jobs) != n || result.Snapshot.Active != 3 {
				t.Fatal(result)
			}
			active := []controlledRun{nextRun(t, starts), nextRun(t, starts), nextRun(t, starts)}
			// Completion order intentionally differs from source order.
			for finished := 0; finished < n; finished++ {
				r := active[len(active)-1]
				active = active[:len(active)-1]
				r.release <- true
				var job Job
				for _, j := range jobs {
					if j.ExecutionID == r.id {
						job = j
						break
					}
				}
				v := awaitState(t, m, job.ID, "completed")
				if filepath.Base(v.FinalPath) != batchName(job.BatchIndex) {
					t.Fatal(v)
				}
				if finished+3 < n {
					next := nextRun(t, starts)
					if next.id != jobs[finished+3].ExecutionID {
						t.Fatal("FIFO", next)
					}
					active = append(active, next)
				}
				if m.Snapshot().Active > 3 {
					t.Fatal("capacity")
				}
			}
			for i := 1; i <= n; i++ {
				if _, err := os.Stat(filepath.Join(dest, batchName(i))); err != nil {
					t.Fatal(i, err)
				}
			}
			if _, err := m.Remove(jobs[4].ID); err != nil {
				t.Fatal(err)
			}
			if m.Snapshot().Jobs[4].BatchIndex != 6 {
				t.Fatal("renumbered")
			}
		})
	}
}
func TestBatchCancelRetryAndMixedFIFO(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
	manual, err := m.Start(Request{URL: "https://host/manual", Directory: dest})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.ImportTXT(txtFixture(t, strings.Repeat("https://host/a\n", 20)), dest)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		nextRun(t, starts)
	}
	job := result.Snapshot.Jobs[1]
	if _, err := m.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	cancelled := awaitState(t, m, job.ID, "cancelled")
	if cancelled.BatchIndex != 1 {
		t.Fatal(cancelled)
	}
	if nextRun(t, starts).id != result.Snapshot.Jobs[3].ExecutionID {
		t.Fatal("cancel FIFO")
	}
	if _, err := m.Retry(job.ID); err != nil {
		t.Fatal(err)
	}
	retry := awaitState(t, m, job.ID, "queued")
	if retry.BatchIndex != 1 || retry.OutputFilename != "1.mp4" || retry.ExecutionID == job.ExecutionID {
		t.Fatal(retry)
	}
	if m.Snapshot().Jobs[0].ID != manual.Jobs[0].ID {
		t.Fatal("manual order")
	}
	m.mu.Lock()
	tail := m.pending[len(m.pending)-1].view.ID
	m.mu.Unlock()
	if tail != job.ID {
		t.Fatal("retry not tail")
	}
}
func TestBatchDestinationCapturedAcrossImports(t *testing.T) {
	m, firstDir, starts, _ := controlledManager(t)
	selectedDir := firstDir
	path := txtFixture(t, "https://host/a")
	first, err := m.ImportTXT(path, selectedDir)
	if err != nil {
		t.Fatal(err)
	}
	selectedDir = t.TempDir()
	second, err := m.ImportTXT(path, selectedDir)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		nextRun(t, starts).release <- true
	}
	for _, expected := range []struct {
		job Job
		dir string
	}{{first.Snapshot.Jobs[0], firstDir}, {second.Snapshot.Jobs[1], selectedDir}} {
		completed := awaitState(t, m, expected.job.ID, "completed")
		if completed.FinalPath != filepath.Join(expected.dir, "1.mp4") {
			t.Fatal("Папка уже принятого batch изменилась", completed)
		}
	}
}
func TestBatchPublicationRaceNoOverwrite(t *testing.T) {
	m, dest, starts, _ := controlledManager(t)
	result, err := m.ImportTXT(txtFixture(t, "https://host/a"), dest)
	if err != nil {
		t.Fatal(err)
	}
	r := nextRun(t, starts)
	path := filepath.Join(dest, "1.mp4")
	os.WriteFile(path, []byte("external"), 0600)
	r.release <- true
	awaitState(t, m, result.Snapshot.Jobs[0].ID, "failed")
	data, _ := os.ReadFile(path)
	if string(data) != "external" {
		t.Fatal("overwritten")
	}
	entries, _ := os.ReadDir(dest)
	if len(entries) != 1 {
		t.Fatal("suffix or pending leaked", entries)
	}
	if _, err := m.Retry(result.Snapshot.Jobs[0].ID); err == nil {
		t.Fatal("retry ignored conflict")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publishBatch(ctx, path, dest, "2.mp4", "id", func(rename func() error) error { return rename() }); err == nil {
		t.Fatal("cancelled publication")
	}
}
