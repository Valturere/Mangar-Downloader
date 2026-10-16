package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"
)

type readinessFlight struct {
	done      chan struct{}
	preflight bool
	err       error
}

type verificationMarker struct {
	Schema     int    `json:"schema"`
	Boot       string `json:"boot"`
	Components string `json:"components"`
}

func (m *Manager) preparationState(state string) {
	m.mu.Lock()
	m.preparation = state
	m.progress.phase(state)
	m.revision++
	m.mu.Unlock()
	m.emit()
}

// Prepare restores the active map from disk without version subprocesses when
// locally intact. Only missing/broken runtime components require installation.
func (m *Manager) Prepare(ctx context.Context) error { return m.automatic(ctx, false) }

// Preflight is the common engine gate, before any media process or secret temp.
// A caller may cancel its wait without cancelling other jobs' shared repair.
func (m *Manager) Preflight(ctx context.Context) error { return m.automatic(ctx, true) }

// A failed wave is shared by queued jobs too, instead of trying the same broken
// repair once per TXT row. Only an explicit Retry/manual recovery starts a new wave.
func (m *Manager) ResetPreflightFailure() {
	m.autoMu.Lock()
	defer m.autoMu.Unlock()
	m.resetFailureLocked()
}
func (m *Manager) resetFailureLocked() {
	if f := m.autoFlight; f != nil {
		select {
		case <-f.done:
			if f.err != nil {
				m.autoFlight = nil
			}
		default:
		}
	}
}
func (m *Manager) RetryPreparation(ctx context.Context) error {
	m.ResetPreflightFailure()
	return m.Preflight(ctx)
}

// WaitAutomatic joins the owned repair after application cancellation, including
// Job Object termination and installer staging cleanup.
func (m *Manager) WaitAutomatic(ctx context.Context) error {
	m.autoMu.Lock()
	f := m.autoFlight
	m.autoMu.Unlock()
	if f == nil {
		return nil
	}
	select {
	case <-f.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) automatic(ctx context.Context, preflight bool) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		m.autoMu.Lock()
		f := m.autoFlight
		if f == nil {
			f = &readinessFlight{done: make(chan struct{}), preflight: preflight}
			m.autoFlight = f
			go func() {
				shared, cancel := context.WithTimeout(m.context, 15*time.Minute)
				defer cancel()
				f.err = m.runAutomatic(shared, preflight)
				m.autoMu.Lock()
				if f.err == nil {
					m.autoFlight = nil
				}
				close(f.done)
				m.autoMu.Unlock()
			}()
		}
		m.autoMu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.done:
			if f.err != nil {
				return f.err
			}
			if !preflight || f.preflight {
				return nil
			}
		}
	}
}

// localState rehydrates authoritative current metadata, validates immutable
// manifest agreement and hashes, and optionally executes the four version probes.
// No Source/HTTP method is reachable here.
func (m *Manager) localState(ctx context.Context, probes bool) (string, error) {
	h := sha256.New()
	var result error
	for _, t := range toolOrder {
		current := filepath.Join(m.toolDir(t), "current.json")
		err := directorySafe(m.paths.Tools, current)
		var man Manifest
		if err == nil {
			man, err = readManifest(current, t)
		}
		if err == nil {
			p := filepath.Join(m.versionDir(t, man.Identity), "manifest.json")
			err = directorySafe(m.paths.Tools, p)
			if err == nil {
				var installed Manifest
				installed, err = readManifest(p, t)
				if err == nil && !reflect.DeepEqual(man, installed) {
					err = fmt.Errorf("current and immutable manifest differ")
				}
			}
		}
		if err == nil {
			err = m.verifyFiles(ctx, man)
		}
		if err == nil && probes {
			err = m.verifyVersion(ctx, man)
		}
		m.mu.Lock()
		delete(m.active, t)
		if err == nil {
			m.active[t] = man
		}
		m.mu.Unlock()
		if err != nil {
			m.record(t, "preflight_local", err)
			state := "corrupted"
			if os.IsNotExist(err) {
				state = "not_installed"
			}
			m.set(Status{Tool: t, State: state, Message: "Требуется подготовка"})
			result = errors.Join(result, err)
		} else {
			b, _ := json.Marshal(man)
			h.Write(b)
			m.installed(man)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	return hex.EncodeToString(h.Sum(nil)), result
}

func (m *Manager) runAutomatic(ctx context.Context, preflight bool) (err error) {
	defer func() {
		if err != nil {
			m.record(Tool("app"), "preparation_failed", err)
			m.preparationState("failed")
		} else {
			m.preparationState("")
		}
	}()
	boot := ""
	if preflight {
		boot, err = m.bootIdentity()
		if err != nil || boot == "" {
			if err == nil {
				err = fmt.Errorf("empty boot identity")
			}
			m.record(YtDlp, "preflight_boot", err)
			return err
		}
		m.autoMu.Lock()
		verified := m.verifiedBoot == boot
		m.autoMu.Unlock()
		if verified {
			return nil
		}
	}
	end, err := m.beginContext(ctx, true)
	if err != nil {
		return err
	}
	defer end()
	m.preparationState("checking")
	if !preflight {
		m.cleanStaging()
	}
	start := time.Now()
	fingerprint, localErr := m.localState(ctx, false)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if preflight && localErr == nil && m.markerMatches(boot, fingerprint) {
		m.autoMu.Lock()
		m.verifiedBoot = boot
		m.autoMu.Unlock()
		return nil
	}
	if preflight && localErr == nil {
		localErr = m.probeActive(ctx)
	}
	if localErr != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.preparationState("preparing")
		// Clear stale success BEFORE repair; failures must never certify this boot.
		if err = m.clearMarker(); err != nil {
			return err
		}
		if err = m.installNeeded(ctx, true); err != nil {
			return err
		}
		m.preparationState("finishing")
		fingerprint, err = m.localState(ctx, true)
		if err != nil {
			return err
		}
	}
	if preflight {
		if err = m.saveMarker(verificationMarker{Schema: 1, Boot: boot, Components: fingerprint}); err != nil {
			return err
		}
		m.autoMu.Lock()
		m.verifiedBoot = boot
		m.autoMu.Unlock()
		if m.log != nil {
			m.log(Tool("app"), "preflight_pass", fmt.Sprintf("local health-check completed in %d ms", time.Since(start).Milliseconds()))
		}
	}
	if !preflight {
		for _, tool := range toolOrder {
			m.cleanVersions(tool)
		}
	}
	return nil
}

func (m *Manager) probeActive(ctx context.Context) error {
	var result error
	for _, t := range toolOrder {
		m.mu.RLock()
		man, ok := m.active[t]
		m.mu.RUnlock()
		if !ok {
			return fmt.Errorf("active component missing")
		}
		if err := m.verifyVersion(ctx, man); err != nil {
			m.record(t, "preflight_probe", err)
			m.mu.Lock()
			delete(m.active, t)
			m.mu.Unlock()
			m.set(Status{Tool: t, State: "corrupted", Message: "Требуется подготовка"})
			result = errors.Join(result, err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return result
}

func (m *Manager) markerPath() string { return filepath.Join(m.paths.Data, "components-verified.json") }
func (m *Manager) markerMatches(boot, components string) bool {
	p := m.markerPath()
	if directorySafe(m.paths.Root, p) != nil {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var mark verificationMarker
	d := json.NewDecoder(io.LimitReader(f, 4097))
	if d.Decode(&mark) != nil {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF && mark.Schema == 1 && mark.Boot == boot && mark.Components == components
}
func (m *Manager) clearMarker() error {
	p := m.markerPath()
	if err := directorySafe(m.paths.Root, p); err != nil {
		return err
	}
	err := os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
func (m *Manager) saveMarker(mark verificationMarker) error {
	p := m.markerPath()
	if err := directorySafe(m.paths.Root, p); err != nil {
		return err
	}
	if err := os.MkdirAll(m.paths.Data, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(m.paths.Data, ".verified-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	err = json.NewEncoder(f).Encode(mark)
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return m.replace(f.Name(), p)
}
