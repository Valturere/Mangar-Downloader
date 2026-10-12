package tools

import (
	"context"
	"path/filepath"
	"sync"
)

// Lease pins an immutable active version for a future engine invocation.
// A job should retain it until ALL child processes have exited.
type Lease struct {
	Paths   map[string]string
	release func()
	once    sync.Once
}

func (l *Lease) Close() { l.once.Do(l.release) }
func (m *Manager) Acquire(ctx context.Context, t Tool) (*Lease, error) {
	m.mu.Lock()
	man, ok := m.active[t]
	if ok {
		m.leases[man.Identity]++
	}
	m.mu.Unlock()
	if !ok {
		return nil, failure("missing", "Рабочий компонент не установлен", "lease unavailable")
	}
	lease := &Lease{Paths: map[string]string{}, release: func() {
		m.mu.Lock()
		m.leases[man.Identity]--
		if m.leases[man.Identity] == 0 {
			delete(m.leases, man.Identity)
		}
		m.mu.Unlock()
	}}
	if err := m.verifyManifest(ctx, man); err != nil {
		lease.Close()
		return nil, err
	}
	for _, exe := range executables(t) {
		lease.Paths[exe] = filepath.Join(m.versionDir(t, man.Identity), exe)
	}
	return lease, nil
}

// Rollback is a core recovery API, not another Settings button.
func (m *Manager) Rollback(ctx context.Context, t Tool) error {
	end, err := m.begin()
	if err != nil {
		return err
	}
	defer end()
	previous, err := readManifest(filepath.Join(m.toolDir(t), "previous.json"), t)
	if err != nil {
		return failure("rollback", "Предыдущая версия компонента недоступна", "previous manifest invalid")
	}
	if err = m.verifyManifest(ctx, previous); err != nil {
		return err
	}
	m.mu.RLock()
	old, ok := m.active[t]
	m.mu.RUnlock()
	if !ok {
		return failure("rollback", "Сначала восстановите текущую установку компонента", "active missing")
	}
	current := filepath.Join(m.toolDir(t), "current.json")
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = writeManifest(current, previous, m.replace); err != nil {
		return err
	}
	if err = m.verifyManifest(ctx, previous); err != nil {
		if restore := writeManifest(current, old, m.replace); restore != nil {
			m.mu.Lock()
			delete(m.active, t)
			m.mu.Unlock()
			return failure("rollback", "Не удалось восстановить указатель компонента. Требуется переустановка", "rollback pointer restoration failed")
		}
		return err
	}
	m.mu.Lock()
	m.active[t] = previous
	m.mu.Unlock()
	m.installed(previous)
	// A failed backup write is diagnostic only: current was already verified.
	if err = writeManifest(filepath.Join(m.toolDir(t), "previous.json"), old, m.replace); err != nil {
		m.record(t, "rollback_backup", err)
	}
	return nil
}
