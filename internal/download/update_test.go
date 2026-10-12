package download

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestUpdateFreezeAdmissionAndRecovery(t *testing.T) {
	for _, state := range []string{"queued", "preparing", "fetching_info", "downloading", "postprocessing", "finalizing", "cancelling", "future", "completed", "failed", "cancelled"} {
		m, dest, _, _ := controlledManager(t)
		m.jobs = []*queuedJob{{view: Job{ID: "fixture", State: state}, request: Request{URL: "https://host/v", Directory: dest}}}
		terminal := state == "completed" || state == "failed" || state == "cancelled"
		if m.IdleForUpdate() != terminal || m.FreezeForUpdate() != terminal {
			t.Fatal("wrong idle policy", state)
		}
		if !terminal {
			if m.jobs[0].view.State != state {
				t.Fatal("active job cancelled")
			}
			m.jobs = nil
			m.Shutdown(context.Background())
			continue
		}
		if m.Snapshot().Closing {
			t.Fatal("freeze permanently closed queue")
		}
		if _, e := m.Start(Request{URL: "https://host/v", Directory: dest}); e == nil {
			t.Fatal("Start during update")
		}
		if _, e := m.Retry("fixture"); e == nil {
			t.Fatal("Retry during update")
		}
		txt := filepath.Join(t.TempDir(), "batch.txt")
		os.WriteFile(txt, []byte("https://host/v\n"), 0600)
		if _, e := m.ImportTXT(txt, dest); e == nil {
			t.Fatal("batch during update")
		}
		m.ResumeAfterUpdateFailure()
		if !m.IdleForUpdate() {
			t.Fatal("queue did not reopen")
		}
		if _, e := m.Start(Request{URL: "https://host/v", Directory: dest}); e != nil {
			t.Fatal(e)
		}
		m.Shutdown(context.Background())
	}
}
func TestUpdateFreezeAndStartRace(t *testing.T) {
	for range 50 {
		m, dest, _, _ := controlledManager(t)
		var wg sync.WaitGroup
		wg.Add(2)
		var frozen bool
		var started error
		go func() { defer wg.Done(); frozen = m.FreezeForUpdate() }()
		go func() { defer wg.Done(); _, started = m.Start(Request{URL: "https://host/v", Directory: dest}) }()
		wg.Wait()
		if frozen && started == nil {
			t.Fatal("admitted after update freeze")
		}
		m.Shutdown(context.Background())
	}
}
