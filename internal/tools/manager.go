package tools

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
)

type Manager struct {
	autoMu       sync.Mutex
	autoFlight   *readinessFlight
	verifiedBoot string
	context      context.Context
	bootIdentity func() (string, error)
	preparation  string
	progress     preparationTracker
	paths        apppaths.Paths
	source       Source
	http         *HTTP
	verifier     Verifier
	mu           sync.RWMutex
	operation    sync.Mutex
	busy         bool
	revision     uint64
	statuses     map[Tool]Status
	active       map[Tool]Manifest
	leases       map[string]int
	callback     func(Snapshot)
	log          func(Tool, string, string)
	replace      func(string, string) error
	rename       func(string, string) error
}
type Options struct {
	Context      context.Context
	BootIdentity func() (string, error)
	Paths        apppaths.Paths
	Source       Source
	HTTP         *HTTP
	Verifier     Verifier
	OnChange     func(Snapshot)
	Log          func(Tool, string, string)
}

func New(o Options) *Manager {
	h := o.HTTP
	if h == nil {
		h = NewHTTP()
	}
	s := o.Source
	if s == nil {
		s = GitHubSource{HTTP: h}
	}
	v := o.Verifier
	if v == nil {
		v = ProcessVerifier{}
	}
	m := &Manager{paths: o.Paths, source: s, http: h, verifier: v, statuses: map[Tool]Status{}, active: map[Tool]Manifest{}, leases: map[string]int{}, callback: o.OnChange, log: o.Log, replace: replaceFile, rename: os.Rename}
	m.context = o.Context
	if m.context == nil {
		m.context = context.Background()
	}
	m.bootIdentity = o.BootIdentity
	if m.bootIdentity == nil {
		m.bootIdentity = windowsBootIdentity
	}
	for _, t := range toolOrder {
		m.statuses[t] = Status{Tool: t, State: "unknown", Message: "Не проверялся"}
	}
	return m
}
func (m *Manager) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := Snapshot{Busy: m.busy, Revision: m.revision, Preparation: m.preparation, Tools: make([]Status, 0, 3)}
	if m.preparation != "" {
		progress := m.progress.view
		s.Progress = &progress
	}
	for _, t := range toolOrder {
		s.Tools = append(s.Tools, m.statuses[t])
	}
	return s
}
func (m *Manager) emit() {
	if m.callback != nil {
		m.callback(m.Snapshot())
	}
}
func (m *Manager) set(s Status) {
	m.mu.Lock()
	if m.preparation == "preparing" {
		m.progress.update(s, time.Now())
	}
	m.statuses[s.Tool] = s
	m.revision++
	m.mu.Unlock()
	m.emit()
}
func (m *Manager) begin() (func(), error) {
	return m.beginContext(context.Background(), false)
}
func (m *Manager) beginContext(ctx context.Context, wait bool) (func(), error) {
	if !filepath.IsAbs(m.paths.Tools) {
		return nil, failure("path", "Не удалось определить абсолютный путь компонентов", "relative tools root")
	}
	for !m.operation.TryLock() {
		if !wait {
			return nil, failure("busy", "Операция с компонентами уже выполняется", "concurrent request")
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if err := ctx.Err(); err != nil {
		m.operation.Unlock()
		return nil, err
	}
	if err := directorySafe(m.paths.Tools, m.paths.Tools); err != nil {
		m.operation.Unlock()
		return nil, err
	}
	if err := os.MkdirAll(m.paths.Tools, 0700); err != nil {
		m.operation.Unlock()
		return nil, err
	}
	unlock, err := acquireLock(m.paths.Tools)
	if err != nil {
		m.operation.Unlock()
		return nil, err
	}
	m.mu.Lock()
	m.busy = true
	m.revision++
	m.mu.Unlock()
	m.emit()
	return func() {
		unlock()
		m.mu.Lock()
		m.busy = false
		m.revision++
		m.mu.Unlock()
		m.emit()
		m.operation.Unlock()
	}, nil
}
func (m *Manager) record(t Tool, phase string, err error) {
	if m.log == nil {
		return
	}
	detail := err.Error()
	var e *ToolError
	if errors.As(err, &e) {
		detail = e.Code + ": " + e.Message + "; " + e.Detail
	}
	m.log(t, phase, detail)
}
func (m *Manager) failed(t Tool, err error) {
	m.record(t, "error", err)
	message := UserError(err)
	var e *ToolError
	if errors.As(err, &e) {
		message = e.Message
	}
	if errors.Is(err, context.Canceled) {
		message = "Установка прервана. Можно повторить"
	}
	m.mu.RLock()
	version := m.statuses[t].Version
	m.mu.RUnlock()
	m.set(Status{Tool: t, State: "failed", Version: version, Message: message})
}
func (m *Manager) toolDir(t Tool) string { return filepath.Join(m.paths.Tools, string(t)) }
func (m *Manager) versionDir(t Tool, id string) string {
	return filepath.Join(m.toolDir(t), "versions", id)
}
func (m *Manager) Detect(ctx context.Context) error {
	end, err := m.begin()
	if err != nil {
		return err
	}
	defer end()
	// Interrupted installs never resume or activate automatically.
	m.cleanStaging()
	for _, t := range toolOrder {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.detectTool(ctx, t)
		m.cleanVersions(t)
	}
	return nil
}
func (m *Manager) detectTool(ctx context.Context, t Tool) {
	m.set(Status{Tool: t, State: "checking", Message: "Проверка…"})
	p := filepath.Join(m.toolDir(t), "current.json")
	manifest, err := readManifest(p, t)
	state := "corrupted"
	message := "Установка повреждена. Требуется переустановка"
	if os.IsNotExist(err) {
		state = "not_installed"
		message = "Не установлен"
	}
	if err == nil {
		err = m.verifyManifest(ctx, manifest)
	}
	m.mu.Lock()
	delete(m.active, t)
	if err == nil {
		m.active[t] = manifest
	}
	m.mu.Unlock()
	if err != nil {
		if state == "corrupted" {
			m.record(t, "local_detection", err)
		}
		m.set(Status{Tool: t, State: state, Message: message})
		return
	}
	m.installed(manifest)
}
func (m *Manager) installed(man Manifest) {
	m.autoMu.Lock()
	m.verifiedBoot = ""
	m.resetFailureLocked()
	m.autoMu.Unlock()
	version := man.Versions[executables(man.Tool)[0]]
	auxiliary := m.auxiliaryDamaged(man)
	m.set(Status{Tool: man.Tool, State: "installed", Version: version, Message: "Установлен", AuxiliaryRepair: auxiliary})
	if m.log != nil {
		m.log(man.Tool, "installed", "Компонент проверен; version="+version+" identity="+man.Identity)
	}
}
func (m *Manager) verifyManifest(ctx context.Context, man Manifest) error {
	if err := m.verifyFiles(ctx, man); err != nil {
		return err
	}
	return m.verifyVersion(ctx, man)
}

func (m *Manager) verifyFiles(ctx context.Context, man Manifest) error {
	dir := m.versionDir(man.Tool, man.Identity)
	if err := directorySafe(m.paths.Tools, dir); err != nil {
		return err
	}
	for name, sum := range man.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if auxiliaryFile(name) {
			continue
		}
		p := filepath.Join(dir, name)
		if err := directorySafe(m.paths.Tools, p); err != nil {
			return err
		}
		got, err := fileHash(p)
		if err != nil {
			return err
		}
		if got != sum {
			return failure("integrity", "Установка компонента повреждена", "local SHA-256 mismatch")
		}
	}
	return nil
}
func (m *Manager) verifyVersion(ctx context.Context, man Manifest) error {
	versions, err := m.verifier.Verify(ctx, man.Tool, m.versionDir(man.Tool, man.Identity))
	if err != nil {
		return err
	}
	for _, exe := range executables(man.Tool) {
		if versions[exe] != man.Versions[exe] {
			return failure("version", "Версия компонента изменилась. Требуется переустановка", "local version mismatch")
		}
	}
	return nil
}

// Schema 1 remains compatible: runtime hashes are all files except explicit notices.
// Unknown files stay critical, so future DLL/runtime additions are never ignored.
func auxiliaryFile(name string) bool {
	name = strings.ToLower(filepath.ToSlash(name))
	return strings.HasPrefix(name, "licenses/") || name == "license.txt" || name == "third_party_licenses.txt" || name == "provenance.txt"
}
func (m *Manager) auxiliaryDamaged(man Manifest) bool {
	dir := m.versionDir(man.Tool, man.Identity)
	for name, sum := range man.Files {
		if !auxiliaryFile(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if directorySafe(m.paths.Tools, path) != nil {
			return true
		}
		got, err := fileHash(path)
		if err != nil || got != sum {
			return true
		}
	}
	return false
}

// CheckUpdates is explicit; healthy startup/preflight never queries latest versions.
func (m *Manager) CheckUpdates(ctx context.Context) error {
	end, err := m.begin()
	if err != nil {
		return err
	}
	defer end()
	for _, t := range toolOrder {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.detectTool(ctx, t)
		m.mu.RLock()
		old, ok := m.active[t]
		localStatus := m.statuses[t]
		m.mu.RUnlock()
		m.set(Status{Tool: t, State: "checking", Version: old.Versions[executables(t)[0]], Message: "Проверка обновлений…"})
		c, e := m.source.Latest(ctx, t)
		if e != nil {
			m.failed(t, e)
			return e
		}
		if !ok {
			m.set(localStatus)
		} else if old.ReleaseID != c.ReleaseID || old.Asset.ID != c.Asset.ID || old.Asset.Digest != c.Asset.Digest {
			m.set(Status{Tool: t, State: "update_available", Version: old.Versions[executables(t)[0]], Message: "Доступно обновление"})
		} else {
			m.installed(old)
		}
	}
	m.manualRecoveryComplete()
	return nil
}

// InstallNeeded installs sequentially and skips working components without updates.
func (m *Manager) InstallNeeded(ctx context.Context) error {
	end, err := m.begin()
	if err != nil {
		return err
	}
	defer end()
	err = m.installNeeded(ctx, false)
	if err == nil {
		m.manualRecoveryComplete()
	}
	return err
}

func (m *Manager) manualRecoveryComplete() {
	m.mu.RLock()
	healthy := len(m.active) == len(toolOrder)
	failed := m.preparation == "failed"
	m.mu.RUnlock()
	if healthy && failed {
		m.ResetPreflightFailure()
		m.preparationState("")
	}
}
func (m *Manager) installNeeded(ctx context.Context, runtimeOnly bool) error {
	m.cleanStaging()
	for _, t := range toolOrder {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.mu.RLock()
		s := m.statuses[t]
		old, ok := m.active[t]
		m.mu.RUnlock()
		if ok {
			if err := m.verifyManifest(ctx, old); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				m.record(t, "health_check", err)
				m.mu.Lock()
				delete(m.active, t)
				m.mu.Unlock()
				ok = false
			}
		}
		if ok && s.State == "installed" && (runtimeOnly || !m.auxiliaryDamaged(old)) {
			continue
		}
		m.set(Status{Tool: t, State: "checking", Version: s.Version, Message: "Получение выпуска…"})
		// Re-resolve on retries: upstream can replace a rolling FFmpeg asset.
		c, e := m.source.Latest(ctx, t)
		if e != nil {
			m.failed(t, e)
			return e
		}
		if ok && old.ReleaseID == c.ReleaseID && old.Asset.ID == c.Asset.ID && old.Asset.Digest == c.Asset.Digest {
			if e = m.verifyManifest(ctx, old); e == nil && !m.auxiliaryDamaged(old) {
				m.installed(old)
				continue
			}
		}
		if e = m.install(ctx, c); e != nil {
			m.failed(t, e)
			return e
		}
	}
	return nil
}
func (m *Manager) install(ctx context.Context, c Candidate) error {
	root := filepath.Join(m.paths.Tools, ".staging")
	if err := directorySafe(m.paths.Tools, root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, "install-")
	if err != nil {
		return err
	}
	defer func() {
		if e := m.removeOwned(stage); e != nil {
			m.record(c.Tool, "cleanup", e)
		}
	}()
	prepared := filepath.Join(stage, "prepared")
	if err = os.Mkdir(prepared, 0700); err != nil {
		return err
	}
	t := c.Tool
	if m.log != nil {
		m.log(t, "download", "Скачивание официального компонента; host=github.com repository="+c.Repository+" tag="+c.Tag+" asset="+c.Asset.Name+" identity="+c.Identity())
	}
	m.set(Status{Tool: t, State: "downloading", Message: "Скачивание…", Total: c.Asset.Size})
	artifact := filepath.Join(stage, "artifact")
	sum, err := m.http.Download(ctx, c.Asset, artifact, maxDownload, func(n, total int64) {
		m.set(Status{Tool: t, State: "downloading", Message: "Скачивание", Bytes: n, Total: total})
	})
	if err != nil {
		return err
	}
	m.set(Status{Tool: t, State: "verifying", Message: "Проверка…"})
	integrity := "HTTPS + размер + структура + запуск"
	if c.Asset.Digest != "" {
		integrity = "GitHub SHA-256 digest"
	}
	if c.Checksum != nil {
		b, e := m.http.Small(ctx, c.Checksum.URL, 1<<20)
		if e != nil {
			return e
		}
		expected, e := checksumFor(b, c.Asset.Name)
		if e != nil {
			return e
		}
		if sum != expected {
			return failure("integrity", "Проверка целостности компонента не пройдена", "checksum asset mismatch")
		}
		integrity += " + release checksum"
	}
	if t == YtDlp {
		err = os.Rename(artifact, filepath.Join(prepared, "yt-dlp.exe"))
	} else {
		err = extract(ctx, artifact, prepared, t)
	}
	if err != nil {
		return err
	}
	if err = m.notices(ctx, c, prepared); err != nil {
		return err
	}
	versions, err := m.verifier.Verify(ctx, t, prepared)
	if err != nil {
		return err
	}
	// Tag/output agreement detects wrong but runnable upstream binaries.
	if t == YtDlp && versions["yt-dlp.exe"] != c.Tag {
		return failure("version", "Версия yt-dlp не соответствует выпуску", "tag/version mismatch")
	}
	if t == Deno && "v"+versions["deno.exe"] != c.Tag {
		return failure("version", "Версия Deno не соответствует выпуску", "tag/version mismatch")
	}
	files, err := hashes(prepared)
	if err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		return err
	}
	man := Manifest{Schema: 1, Tool: t, Identity: c.Identity() + "-" + hex.EncodeToString(random), Repository: c.Repository, ReleaseID: c.ReleaseID, Tag: c.Tag, Asset: c.Asset, InstalledAt: time.Now().UTC(), Executables: map[string]string{}, Versions: versions, SHA256: sum, Integrity: integrity, Files: files}
	man.Asset.URL = "" // Manifest never persists a download/redirect URL.
	for _, exe := range executables(t) {
		man.Executables[exe] = exe
	}
	m.set(Status{Tool: t, State: "installing", Message: "Установка…"})
	dest := m.versionDir(t, man.Identity)
	if err = directorySafe(m.paths.Tools, dest); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = m.rename(prepared, dest); err != nil {
		return err
	}
	// Installation is immutable; a failed activation leaves an unreferenced version,
	// safely collected after a later successful operation/startup.
	if err = writeManifest(filepath.Join(dest, "manifest.json"), man, m.replace); err != nil {
		return err
	}
	if err = m.verifyManifest(ctx, man); err != nil {
		return err
	}
	m.mu.RLock()
	old, hadOld := m.active[t]
	m.mu.RUnlock()
	if hadOld {
		if err = writeManifest(filepath.Join(m.toolDir(t), "previous.json"), old, m.replace); err != nil {
			return err
		}
	}
	current := filepath.Join(m.toolDir(t), "current.json")
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = writeManifest(current, man, m.replace); err != nil {
		return err
	}
	// Activation/rollback must finish even if shutdown cancels the download context.
	checkCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err = m.verifyManifest(checkCtx, man); err != nil {
		var rollback error
		if hadOld {
			rollback = writeManifest(current, old, m.replace)
		} else {
			rollback = os.Remove(current)
		}
		if rollback != nil {
			m.mu.Lock()
			delete(m.active, t)
			m.mu.Unlock()
			return failure("rollback", "Не удалось восстановить указатель компонента. Требуется переустановка", fmt.Sprintf("activation=%v rollback=%v", err, rollback))
		}
		return err
	}
	m.mu.Lock()
	m.active[t] = man
	m.mu.Unlock()
	m.installed(man)
	m.cleanVersions(t)
	return nil
}
func (m *Manager) notices(ctx context.Context, c Candidate, dir string) error {
	files := map[string]string{}
	if c.Tool == YtDlp {
		files["LICENSE.txt"] = "LICENSE"
		files["THIRD_PARTY_LICENSES.txt"] = "THIRD_PARTY_LICENSES.txt"
	}
	if c.Tool == Deno {
		files["LICENSE.txt"] = "LICENSE.md"
	}
	for name, source := range files {
		u := "https://raw.githubusercontent.com/" + c.Repository + "/" + c.Tag + "/" + source
		b, err := m.http.Small(ctx, u, 2<<20)
		if err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			return err
		}
	}
	notice := fmt.Sprintf("Project: %s\nRelease: %s\nRelease ID: %d\nAsset: %s (ID %d)\nUpstream/source: https://github.com/%s\nRelease/source metadata: https://github.com/%s/releases/tag/%s\nDownloaded directly from upstream; not bundled with this application.\n", c.Repository, c.Tag, c.ReleaseID, c.Asset.Name, c.Asset.ID, c.Repository, c.Repository, c.Tag)
	if c.Tool == FFmpeg {
		notice += "FFmpeg GPL build; license texts are preserved under licenses/. Build recipes and corresponding upstream source references: https://github.com/yt-dlp/FFmpeg-Builds ; https://github.com/FFmpeg/FFmpeg\n"
	}
	return os.WriteFile(filepath.Join(dir, "PROVENANCE.txt"), []byte(notice), 0600)
}
func (m *Manager) removeOwned(p string) error {
	if err := directorySafe(m.paths.Tools, p); err != nil {
		return err
	}
	if filepath.Clean(p) == filepath.Clean(m.paths.Tools) {
		return fmt.Errorf("Нельзя удалять корень tools")
	}
	if err := filepath.WalkDir(p, func(entry string, _ os.DirEntry, e error) error {
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		return directorySafe(m.paths.Tools, entry)
	}); err != nil {
		return err
	}
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		err = os.RemoveAll(p)
		if err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}
func (m *Manager) cleanStaging() {
	dir := filepath.Join(m.paths.Tools, ".staging")
	if directorySafe(m.paths.Tools, dir) != nil {
		return
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if err := m.removeOwned(p); err != nil {
			m.record("", "staging_cleanup", err)
		}
	}
}
func (m *Manager) cleanVersions(t Tool) {
	keep := map[string]bool{}
	current, previous, err := m.cleanupPointers(t)
	if err != nil {
		m.cleanupAudit(t, "gc_skip", "reason=manifest_read_error")
		return
	}
	keep[current], keep[previous] = true, true
	m.mu.RLock()
	for identity, count := range m.leases {
		if count > 0 {
			keep[identity] = true
		}
	}
	m.mu.RUnlock()
	root := filepath.Join(m.toolDir(t), "versions")
	if directorySafe(m.paths.Tools, root) != nil {
		return
	}
	entries, _ := os.ReadDir(root)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if !e.IsDir() || !identityPattern.MatchString(e.Name()) || keep[e.Name()] {
			continue
		}
		m.cleanVersion(t, e.Name())
	}
}

// Callers hold manager.lock from begin through the entire cleanup. The initial
// keep-set only avoids work; it never authorizes deletion of a candidate.
func (m *Manager) cleanupPointers(t Tool) (current, previous string, err error) {
	for _, pointer := range []struct {
		name string
		id   *string
	}{{"current.json", &current}, {"previous.json", &previous}} {
		path := filepath.Join(m.toolDir(t), pointer.name)
		if err = directorySafe(m.paths.Tools, path); err != nil {
			return
		}
		var man Manifest
		man, err = readManifest(path, t)
		if os.IsNotExist(err) {
			err = nil // Interrupted installs can leave genuine orphans without pointers.
		} else if err != nil {
			return
		} else {
			*pointer.id = man.Identity
		}
	}
	return
}

func (m *Manager) cleanupAudit(t Tool, phase, detail string) {
	if m.log != nil {
		m.log(t, phase, detail)
	}
}

func (m *Manager) guardVersion(t Tool, identity string, deleting bool) (func(), error) {
	if !identityPattern.MatchString(identity) {
		return nil, fmt.Errorf("Некорректная identity для блокировки версии")
	}
	path := filepath.Join(m.paths.Tools, ".leases", string(t), identity+".lock")
	if err := directorySafe(m.paths.Tools, path); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return versionGuard(path, deleting)
}

func (m *Manager) cleanVersion(t Tool, identity string) {
	if !identityPattern.MatchString(identity) {
		return
	}
	dir := m.versionDir(t, identity)
	if err := directorySafe(m.paths.Tools, dir); err != nil {
		m.record(t, "old_version_cleanup", err)
		return
	}
	// Exclude leases in ALL Manager instances/processes before touching children.
	if _, err := os.Stat(dir); err != nil {
		return
	}
	unlock, err := m.guardVersion(t, identity, true)
	if err != nil {
		if !os.IsNotExist(err) {
			m.cleanupAudit(t, "gc_skip", "identity="+identity+" reason=leased_or_unavailable")
		}
		return
	}
	defer unlock()
	if err = versionUnlocked(dir, t); err != nil {
		m.record(t, "old_version_cleanup", err)
		return
	}
	// Acquire increments the local lease count under mu. Keep that exclusion
	// through removeOwned, including retries, rather than relying on a snapshot.
	m.mu.RLock()
	defer m.mu.RUnlock()
	current, previous, err := m.cleanupPointers(t)
	if err != nil {
		m.cleanupAudit(t, "gc_skip", "identity="+identity+" reason=manifest_read_error")
		return
	}
	leased := m.leases[identity] > 0
	if identity == current || identity == previous || leased {
		m.cleanupAudit(t, "gc_skip", fmt.Sprintf("identity=%s reason=protected current=%s previous=%s leased=%t", identity, current, previous, leased))
		return
	}
	m.cleanupAudit(t, "gc_delete", fmt.Sprintf("identity=%s reason=orphan current=%s previous=%s leased=%t", identity, current, previous, leased))
	if err = m.removeOwned(dir); err != nil {
		m.record(t, "old_version_cleanup", err)
		return
	}
	m.cleanupAudit(t, "gc_deleted", "identity="+identity+" result=deleted")
}

// Resolve returns one immutable version directory, never a system PATH result.
// ResolveFFmpegPair prevents ffmpeg/ffprobe crossing an activation boundary.
func (m *Manager) Resolve(ctx context.Context, t Tool, exe string) (string, error) {
	m.mu.RLock()
	man, ok := m.active[t]
	m.mu.RUnlock()
	if !ok || man.Executables[exe] != exe {
		return "", failure("missing", "Рабочий компонент не установлен", "resolver unavailable")
	}
	if err := m.verifyManifest(ctx, man); err != nil {
		return "", err
	}
	return filepath.Join(m.versionDir(t, man.Identity), exe), nil
}
func (m *Manager) ResolveYtDlp(ctx context.Context) (string, error) {
	return m.Resolve(ctx, YtDlp, "yt-dlp.exe")
}
func (m *Manager) ResolveDeno(ctx context.Context) (string, error) {
	return m.Resolve(ctx, Deno, "deno.exe")
}
func (m *Manager) ResolveFFmpegPair(ctx context.Context) (string, string, error) {
	m.mu.RLock()
	man, ok := m.active[FFmpeg]
	m.mu.RUnlock()
	if !ok {
		return "", "", failure("missing", "FFmpeg / ffprobe не установлен", "resolver unavailable")
	}
	if err := m.verifyManifest(ctx, man); err != nil {
		return "", "", err
	}
	dir := m.versionDir(FFmpeg, man.Identity)
	return filepath.Join(dir, "ffmpeg.exe"), filepath.Join(dir, "ffprobe.exe"), nil
}

// UserError removes OS paths and raw external output at the Wails boundary.
func UserError(err error) string {
	var e *ToolError
	if errors.As(err, &e) {
		return e.Message
	}
	if errors.Is(err, context.Canceled) {
		return "Установка прервана"
	}
	if message := filesystemMessage(err); message != "" {
		return message
	}
	return "Операция с компонентами не выполнена. Проверьте доступ к диску и повторите"
}
