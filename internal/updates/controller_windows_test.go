//go:build windows

package updates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func controllerFixture(t *testing.T, options fixtureOptions) *Controller {
	t.Helper()
	client, _, _ := fixtureClient(t, options)
	c := &Controller{remote: client, data: t.TempDir(), root: filepath.Join(t.TempDir(), "updates"), now: time.Now, state: Snapshot{Version: "0.9.0", Phase: "idle"}}
	t.Cleanup(func() { c.Close(context.Background()) })
	return c
}
func TestAnonymousAutomaticTTLAndManualBypass(t *testing.T) {
	c := controllerFixture(t, fixtureOptions{})
	now := time.Now()
	c.now = func() time.Time { return now }
	if e := c.Check(true); e != nil || c.Snapshot().Latest != "1.0.0" {
		t.Fatal(e)
	}
	client := c.remote.(*Client)
	client.base = "invalid" // a TTL skip must perform no request
	if e := c.Check(true); e != nil {
		t.Fatal("TTL not respected", e)
	}
	if e := c.Check(false); e == nil {
		t.Fatal("manual check incorrectly skipped")
	}
	b, e := os.ReadFile(filepath.Join(c.data, "update-check.json"))
	if e != nil || strings.Contains(string(b), fixturePrivateBody) || strings.Contains(string(b), "Authorization") {
		t.Fatal("timestamp persistence", e)
	}
}
func TestUpdateControllerCancellationAndConcurrentAdmission(t *testing.T) {
	c := controllerFixture(t, fixtureOptions{})
	ctx, e := c.begin("downloading")
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := c.begin("checking"); e == nil {
				t.Error("parallel operation accepted")
			}
			_ = c.Snapshot()
		}()
	}
	c.Cancel()
	wg.Wait()
	if ctx.Err() == nil {
		t.Fatal("cancel not propagated")
	}
	c.finish(ctx.Err(), "")
	c.Close(context.Background())
	if _, e := c.begin("checking"); e == nil {
		t.Fatal("operation after shutdown")
	}
}
func TestStageCleanupOwnershipAndStaleIsolation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Обновления & 100%")
	dir, e := NewStage(root)
	if e != nil {
		t.Fatal(e)
	}
	unknown := filepath.Join(dir, "user-download.mp4")
	os.WriteFile(unknown, []byte("keep"), 0600)
	if CleanupStage(dir, root) == nil {
		t.Fatal("unknown file accepted")
	}
	if _, e = os.Stat(unknown); e != nil {
		t.Fatal("foreign file removed")
	}
	os.Remove(unknown)
	os.WriteFile(filepath.Join(dir, InstallerName("1.0.0")+".part"), []byte("partial"), 0600)
	CleanupStale(root, time.Now().Add(2*time.Hour))
	if _, e = os.Stat(dir); e != nil {
		t.Fatal("fresh stage removed")
	}
	CleanupStale(root, time.Now().Add(25*time.Hour))
	if _, e = os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("stale owned stage retained", e)
	}
	outside := t.TempDir()
	if CleanupStage(outside, root) == nil {
		t.Fatal("outside stage accepted")
	}
}
