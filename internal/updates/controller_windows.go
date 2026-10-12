//go:build windows

package updates

import (
	"YTDLP-GUI-Mangar/internal/installation"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// No DTO or callback contains a credential, asset URL or response body.
type Snapshot struct {
	Revision         uint64 `json:"revision"`
	Version          string `json:"version"`
	Latest           string `json:"latest"`
	Busy             bool   `json:"busy"`
	Phase            string `json:"phase"`
	Message          string `json:"message"`
	Error            string `json:"error"`
	InstallSupported bool   `json:"installSupported"`
}
type remote interface {
	Check(context.Context) (*Candidate, error)
	Download(context.Context, Candidate, string) (string, error)
}
type ControllerOptions struct {
	Version, Data, InstallDir, StagingRoot string
	OnChange                               func(Snapshot)
}
type Controller struct {
	mu                            sync.Mutex
	state                         Snapshot
	remote                        remote
	data, installDir, root, stage string
	candidate                     *Candidate
	checked                       time.Time
	cancel                        context.CancelFunc
	done                          chan struct{}
	closed                        bool
	callback                      func(Snapshot)
	now                           func() time.Time
}

func NewController(o ControllerOptions) *Controller {
	c := &Controller{remote: NewClient(o.Version), data: o.Data, installDir: o.InstallDir, root: o.StagingRoot, callback: o.OnChange, now: time.Now}
	c.state = Snapshot{Version: o.Version, Phase: "idle", Message: "Проверьте наличие обновлений"}
	expected, e := defaultInstallDir()
	if e == nil && strings.EqualFold(o.InstallDir, expected) {
		metadata, e := installation.ReadMetadata(o.InstallDir)
		c.state.InstallSupported = e == nil && metadata.Version == o.Version && !metadata.SourceDirty
	}
	var stamp struct {
		Checked time.Time `json:"checked"`
	}
	if b, e := os.ReadFile(filepath.Join(c.data, "update-check.json")); e == nil && len(b) < 1024 && json.Unmarshal(b, &stamp) == nil {
		c.checked = stamp.Checked
	}
	var result Result
	resultFile := filepath.Join(c.data, "update-result.json")
	if b, e := os.ReadFile(resultFile); e == nil && len(b) < 1024 && json.Unmarshal(b, &result) == nil && result.Schema == 1 && result.ProductID == installation.ProductID {
		if result.Code == "success" && result.Version == o.Version {
			c.state.Message = "Приложение обновлено до " + o.Version
		} else {
			c.state.Error = "Предыдущее обновление не завершилось. Приложение доступно; проверьте версию и повторите попытку"
		}
		_ = os.Remove(resultFile)
	}
	return c
}
func (c *Controller) Snapshot() Snapshot { c.mu.Lock(); defer c.mu.Unlock(); return c.state }
func (c *Controller) publish() {
	c.mu.Lock()
	c.state.Revision++
	s := c.state
	c.mu.Unlock()
	if c.callback != nil {
		c.callback(s)
	}
}
func (c *Controller) begin(phase string) (context.Context, error) {
	c.mu.Lock()
	if c.closed || c.state.Busy {
		c.mu.Unlock()
		return nil, errors.New("Операция обновления уже выполняется или приложение завершается")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	c.cancel = cancel
	c.done = make(chan struct{})
	c.state.Busy = true
	c.state.Phase = phase
	c.state.Error = ""
	c.state.Message = ""
	c.mu.Unlock()
	c.publish()
	return ctx, nil
}
func userError(e error) string {
	for _, known := range []error{ErrAccess, ErrNoRelease, ErrRate, ErrService, ErrNetwork, ErrMetadata, ErrIntegrity, ErrDownload, ErrRedirect, ErrHelper} {
		if errors.Is(e, known) {
			return known.Error()
		}
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return "Операция обновления отменена или превысила время ожидания"
	}
	return "Не удалось выполнить обновление. Повторите попытку"
}
func (c *Controller) finish(e error, message string) {
	c.mu.Lock()
	c.cancel()
	c.cancel = nil
	c.state.Busy = false
	c.state.Phase = "idle"
	c.state.Message = message
	if e != nil {
		c.state.Message = ""
		c.state.Error = userError(e)
	}
	close(c.done)
	c.mu.Unlock()
	c.publish()
}
func (c *Controller) Check(automatic bool) error {
	c.mu.Lock()
	skip := automatic && (c.closed || c.state.Busy || (!c.checked.IsZero() && c.now().Sub(c.checked) < 24*time.Hour && c.now().Sub(c.checked) >= 0))
	c.mu.Unlock()
	if skip {
		return nil
	}
	ctx, e := c.begin("checking")
	if e != nil {
		return e
	}
	candidate, e := c.remote.Check(ctx)
	c.mu.Lock()
	c.candidate = candidate
	c.state.Latest = ""
	if candidate != nil {
		c.state.Latest = candidate.Manifest.Version
	}
	c.checked = c.now().UTC()
	stamp := c.checked
	c.mu.Unlock()
	// Persist only the attempt timestamp, never authentication or remote bodies.
	if os.MkdirAll(c.data, 0700) == nil {
		b, _ := json.Marshal(struct {
			Checked time.Time `json:"checked"`
		}{stamp})
		_ = os.WriteFile(filepath.Join(c.data, "update-check.json"), b, 0600)
	}
	message := "Установлена актуальная версия"
	if candidate != nil {
		message = "Доступна версия " + candidate.Manifest.Version
	}
	c.finish(e, message)
	return e
}

// Prepare re-fetches release metadata at the explicit install click. A stale
// candidate is never sufficient authority to run a cached executable.
func (c *Controller) Prepare() (plan string, e error) {
	ctx, e := c.begin("downloading")
	if e != nil {
		return "", e
	}
	defer func() { c.finish(e, "") }()
	c.mu.Lock()
	supported := c.state.InstallSupported
	oldStage := c.stage
	c.stage = ""
	c.mu.Unlock()
	if !supported {
		return "", ErrHelper
	}
	if oldStage != "" {
		_ = CleanupStage(oldStage, c.root)
	}
	candidate, e := c.remote.Check(ctx)
	if e != nil {
		return "", e
	}
	if candidate == nil {
		return "", ErrMetadata
	}
	dir, e := NewStage(c.root)
	if e != nil {
		return "", e
	}
	good := false
	defer func() {
		if !good {
			_ = CleanupStage(dir, c.root)
		}
	}()
	if _, e = c.remote.Download(ctx, *candidate, dir); e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	plan, e = PreparePlan(dir, c.installDir, c.state.Version, candidate.Manifest)
	if e != nil {
		return "", e
	}
	if e = ctx.Err(); e != nil {
		return "", e
	}
	c.mu.Lock()
	c.stage = dir
	c.candidate = candidate
	c.state.Latest = candidate.Manifest.Version
	c.mu.Unlock()
	good = true
	return plan, nil
}
func (c *Controller) Cancel() {
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
}
func (c *Controller) Close(ctx context.Context) {
	c.mu.Lock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	done := c.done
	busy := c.state.Busy
	c.mu.Unlock()
	if busy {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}
func (c *Controller) DiscardPrepared() {
	c.mu.Lock()
	dir := c.stage
	c.stage = ""
	c.mu.Unlock()
	if dir != "" {
		_ = CleanupStage(dir, c.root)
	}
}
func (c *Controller) AutoStart() { CleanupStale(c.root, c.now()); _ = c.Check(true) }
