package download

import (
	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/process"
	"YTDLP-GUI-Mangar/internal/tools"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const MaxActive = 3
const metricTTL = 3 * time.Second

type Options struct {
	Preflight func(context.Context) error
	Paths     apppaths.Paths
	Tools     ToolProvider
	Run       Runner
	// OnChange runs outside the manager lock and may be concurrent. Consumers
	// must be thread-safe and use Revision to reject out-of-order snapshots.
	OnChange      func(Snapshot)
	Now           func() time.Time
	CookieMode    func() string
	DetectBrowser func() string
}
type queuedJob struct {
	view      Job
	request   Request
	cancel    context.CancelFunc
	done      chan struct{}
	committed bool
}
type Manager struct {
	preflight      func(context.Context) error
	retryPreflight func()
	mu             sync.Mutex
	paths          apppaths.Paths
	run            Runner
	acquire        func(context.Context, tools.Tool) (map[string]string, func(), error)
	callback       func(Snapshot)
	now            func() time.Time
	jobs           []*queuedJob
	pending        []*queuedJob
	active         int
	sequence       uint64
	revision       uint64
	ctx            context.Context
	cancel         context.CancelFunc
	closing        bool
	updatePaused   bool
	wg             sync.WaitGroup
	cookieMode     func() string
	detectBrowser  func() string
}
type execution struct {
	manager *Manager
	job     *queuedJob
	ctx     context.Context
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func New(o Options) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	run := o.Run
	if run == nil {
		run = process.RunStreaming
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	m := &Manager{paths: o.Paths, run: run, callback: o.OnChange, now: now, ctx: ctx, cancel: cancel}
	m.preflight = o.Preflight
	if m.preflight == nil {
		if ready, ok := o.Tools.(interface{ Preflight(context.Context) error }); ok {
			m.preflight = ready.Preflight
		}
	}
	if ready, ok := o.Tools.(interface{ ResetPreflightFailure() }); ok {
		m.retryPreflight = ready.ResetPreflightFailure
	}
	m.cookieMode = o.CookieMode
	m.detectBrowser = o.DetectBrowser
	if m.detectBrowser == nil {
		m.detectBrowser = detectCookieBrowser
	}
	m.acquire = func(ctx context.Context, t tools.Tool) (map[string]string, func(), error) {
		if o.Tools == nil {
			return nil, nil, errors.New("Нет managed tools")
		}
		l, err := o.Tools.Acquire(ctx, t)
		if err != nil {
			return nil, nil, err
		}
		return l.Paths, l.Close, nil
	}
	return m
}
func copyJob(s Job) Job {
	clone := func(v *float64) *float64 {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	stamp := func(v *time.Time) *time.Time {
		if v == nil {
			return nil
		}
		n := *v
		return &n
	}
	s.Percent = clone(s.Percent)
	s.Speed = clone(s.Speed)
	s.ETA = clone(s.ETA)
	s.CreatedAt = s.CreatedAt.UTC()
	s.StartedAt = stamp(s.StartedAt)
	s.FinishedAt = stamp(s.FinishedAt)
	s.SpeedAt = stamp(s.SpeedAt)
	s.ETAAt = stamp(s.ETAAt)
	return s
}
func (m *Manager) snapshotLocked() Snapshot {
	s := Snapshot{Revision: m.revision, Active: m.active, Closing: m.closing, Jobs: make([]Job, 0, len(m.jobs))}
	for _, j := range m.jobs {
		v := copyJob(j.view)
		expireMetrics(&v, m.now())
		s.Jobs = append(s.Jobs, v)
	}
	return s
}
func (m *Manager) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshotLocked() }
func (m *Manager) notify(s Snapshot) {
	if m.callback != nil {
		m.callback(s)
	}
}

// The sole admission point reserves slots under the same lock as FIFO mutation.
func (m *Manager) scheduleLocked() {
	for !m.closing && !m.updatePaused && m.active < MaxActive && len(m.pending) > 0 {
		j := m.pending[0]
		m.pending[0] = nil
		m.pending = m.pending[1:]
		ctx, cancel := context.WithCancel(m.ctx)
		j.cancel = cancel
		j.done = make(chan struct{})
		j.committed = false
		j.view.State = "preparing"
		j.view.Active = true
		j.view.Message = "Подготовка…"
		now := m.now()
		j.view.StartedAt = &now
		m.active++
		m.revision++
		m.wg.Add(1)
		e := &execution{manager: m, job: j, ctx: ctx}
		done, request, executionID := j.done, j.request, j.view.ExecutionID
		go func() {
			defer m.wg.Done()
			defer close(done)
			defer cancel()
			e.execute(request, executionID)
		}()
	}
}
func (m *Manager) Start(r Request) (Snapshot, error) {
	r.cookieMode = m.admissionCookieMode()
	source, err := ValidateURL(r.URL)
	if err != nil {
		return Snapshot{}, err
	}
	r.URL = source
	r, err = prepareHTTPRequest(r)
	if err != nil {
		return Snapshot{}, err
	}
	if r.Directory == "" {
		return Snapshot{}, errors.New("Выберите папку для загрузки.")
	}
	if !filepath.IsAbs(r.Directory) {
		return Snapshot{}, errors.New("Нет доступа к выбранной папке")
	}
	stat, err := os.Stat(r.Directory)
	if err != nil || !stat.IsDir() {
		return Snapshot{}, errors.New("Нет доступа к выбранной папке")
	}
	id, err := newID()
	if err != nil {
		return Snapshot{}, errors.New("Не удалось создать задание")
	}
	m.mu.Lock()
	if m.updatePaused {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Приложение готовится к обновлению")
	}
	if m.closing {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Приложение завершает работу")
	}
	m.sequence++
	title, _ := browserTitle("", r)
	j := &queuedJob{request: r, view: Job{ID: id, ExecutionID: id, Sequence: m.sequence, Attempt: 1, CreatedAt: m.now(), Source: safeSource(source), Title: title, State: "queued", Message: "В очереди"}}
	m.jobs = append(m.jobs, j)
	m.pending = append(m.pending, j)
	m.revision++
	m.scheduleLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(s)
	return s, nil
}
func (m *Manager) findLocked(id string) *queuedJob {
	for _, j := range m.jobs {
		if j.view.ID == id {
			return j
		}
	}
	return nil
}
func (m *Manager) removePendingLocked(j *queuedJob) {
	for i, v := range m.pending {
		if v == j {
			m.pending = slices.Delete(m.pending, i, i+1)
			return
		}
	}
}
func (m *Manager) Cancel(id string) (Snapshot, error) {
	m.mu.Lock()
	j := m.findLocked(id)
	if j == nil {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Задание не найдено")
	}
	if j.view.State == "queued" {
		m.removePendingLocked(j)
		j.view.State = "cancelled"
		j.view.Message = "Отменено"
		now := m.now()
		j.view.FinishedAt = &now
		m.revision++
	} else if j.view.Active && !j.committed && j.view.State != "cancelling" {
		j.view.State = "cancelling"
		j.view.Message = "Отмена…"
		clearMetrics(&j.view)
		j.cancel()
		m.revision++
	}
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(s)
	return s, nil
}
func (m *Manager) Retry(id string) (Snapshot, error) {
	executionID, err := newID()
	if err != nil {
		return Snapshot{}, errors.New("Не удалось создать попытку")
	}
	m.mu.Lock()
	j := m.findLocked(id)
	if m.updatePaused {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Приложение готовится к обновлению")
	}
	if m.closing {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Приложение завершает работу")
	}
	if j == nil || j.view.InvalidSource || (j.view.State != "failed" && j.view.State != "cancelled") {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Повтор доступен для ошибочных и отменённых заданий")
	}
	if j.view.BatchIndex > 0 {
		if err := m.batchConflictsLocked(j.request.Directory, []BatchEntry{{Index: j.view.BatchIndex}}, j); err != nil {
			m.mu.Unlock()
			return Snapshot{}, err
		}
	}
	if m.retryPreflight != nil {
		m.retryPreflight()
	}
	j.view.ExecutionID = executionID
	j.view.Attempt++
	j.view.State = "queued"
	j.view.Message = "В очереди"
	j.view.StartedAt = nil
	j.view.FinishedAt = nil
	j.view.Error = ""
	j.view.FinalPath = ""
	j.view.FormatID = ""
	clearMetrics(&j.view)
	m.pending = append(m.pending, j)
	m.revision++
	m.scheduleLocked()
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(s)
	return s, nil
}
func (m *Manager) Remove(id string) (Snapshot, error) {
	m.mu.Lock()
	j := m.findLocked(id)
	if j == nil {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Задание не найдено")
	}
	if j.view.Active {
		m.mu.Unlock()
		return Snapshot{}, errors.New("Сначала отмените загрузку")
	}
	m.removePendingLocked(j)
	j.request = Request{} // Release secret context on explicit removal.
	for i, v := range m.jobs {
		if v == j {
			m.jobs = slices.Delete(m.jobs, i, i+1)
			break
		}
	}
	m.revision++
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(s)
	return s, nil
}
func (m *Manager) CompletedPath(id string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.findLocked(id)
	if j == nil || j.view.State != "completed" || j.view.FinalPath == "" {
		return "", errors.New("Готовый файл не найден")
	}
	return j.view.FinalPath, nil
}

// StopAdmission is the synchronous boundary of confirmed Exit. It does not
// invoke callbacks, so the application gate can close atomically with it.
func (m *Manager) StopAdmission() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopAdmissionLocked()
}

// FreezeForUpdate checks idleness and closes every admission path under one
// scheduler lock. It does not cancel jobs or permanently shut down the manager.
func (m *Manager) FreezeForUpdate() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.updatePaused || m.active != 0 || len(m.pending) != 0 {
		return false
	}
	for _, j := range m.jobs {
		if j.view.State != "completed" && j.view.State != "failed" && j.view.State != "cancelled" {
			return false
		}
	}
	m.updatePaused = true
	return true
}
func (m *Manager) ResumeAfterUpdateFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closing {
		m.updatePaused = false
	}
}
func (m *Manager) IdleForUpdate() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closing || m.updatePaused || m.active != 0 || len(m.pending) != 0 {
		return false
	}
	for _, j := range m.jobs {
		if j.view.State != "completed" && j.view.State != "failed" && j.view.State != "cancelled" {
			return false
		}
	}
	return true
}

// StopIfIdle couples the no-confirmation decision with the admission boundary:
// a racing Start cannot slip in after the application observed an empty queue.
func (m *Manager) StopIfIdle() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		switch j.view.State {
		case "completed", "failed", "cancelled":
		default:
			return false
		}
	}
	m.stopAdmissionLocked()
	return true
}

func (m *Manager) stopAdmissionLocked() {
	if m.closing {
		return
	}
	m.closing = true
	m.cancel()
	m.pending = nil
	m.revision++
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.StopAdmission()
	m.notify(m.Snapshot())
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		m.mu.Lock()
		for _, j := range m.jobs {
			j.request = Request{}
		}
		m.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *execution) snapshot() Job {
	e.manager.mu.Lock()
	defer e.manager.mu.Unlock()
	return copyJob(e.job.view)
}
func (e *execution) change(edit func(*Job)) {
	m := e.manager
	m.mu.Lock()
	if e.job.view.State == "cancelling" {
		m.mu.Unlock()
		return
	}
	edit(&e.job.view)
	m.revision++
	s := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(s)
}
func clearMetrics(s *Job) {
	s.Percent = nil
	s.Speed = nil
	s.ETA = nil
	s.SpeedAt = nil
	s.ETAAt = nil
}
func expireMetrics(s *Job, now time.Time) {
	if s.SpeedAt != nil && now.Sub(*s.SpeedAt) >= metricTTL {
		s.Speed = nil
		s.SpeedAt = nil
	}
	if s.ETAAt != nil && now.Sub(*s.ETAAt) >= metricTTL {
		s.ETA = nil
		s.ETAAt = nil
	}
}
func (e *execution) phase(state, message string) {
	e.change(func(s *Job) { s.State = state; s.Message = message; clearMetrics(s) })
}
func (e *execution) progress(p progress, percent *float64, now time.Time, emit bool) {
	m := e.manager
	m.mu.Lock()
	if e.job.view.State == "cancelling" {
		m.mu.Unlock()
		return
	}
	s := &e.job.view
	s.State = "downloading"
	s.Message = "Скачивание"
	s.Percent = percent
	s.FormatID = p.FormatID
	if p.Speed != nil {
		s.Speed = p.Speed
		stamp := now
		s.SpeedAt = &stamp
	}
	if p.ETA != nil {
		s.ETA = p.ETA
		stamp := now
		s.ETAAt = &stamp
	}
	expireMetrics(s, now)
	m.revision++
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	if emit {
		m.notify(snapshot)
	}
}

// Cancellation and the irreversible rename share a lock. Once publication wins,
// Cancel is a no-op; cleanup still holds the slot until completed is announced.
func (e *execution) commit(rename func() error) error {
	m := e.manager
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := e.ctx.Err(); err != nil {
		return err
	}
	if err := rename(); err != nil {
		return err
	}
	e.job.committed = true
	return nil
}
func (e *execution) finish(err error, message string) {
	m := e.manager
	m.mu.Lock()
	s := &e.job.view
	if err == nil {
		e.job.request.Browser = nil
		if e.job.request.HTTPContext != nil && e.job.request.HTTPContext.Captured {
			e.job.request.HTTPContext = nil
		}
		s.State = "completed"
		s.Message = "Готово"
	} else if e.ctx.Err() != nil {
		s.State = "cancelled"
		s.Message = "Отменено"
	} else {
		s.State = "failed"
		s.Message = message
		s.Error = message
	}
	s.Active = false
	clearMetrics(s)
	now := m.now()
	s.FinishedAt = &now
	m.active--
	m.revision++
	m.scheduleLocked()
	snapshot := m.snapshotLocked()
	m.mu.Unlock()
	m.notify(snapshot)
}

func (e *execution) execute(r Request, id string) {
	var log *jobLog
	var owner *os.File
	var releases []func()
	temp := filepath.Join(e.manager.paths.Temp, id)
	message := "Не удалось подготовить загрузку"
	var jobErr error
	defer func() {
		if classified := filesystemFailure(jobErr); classified != "" {
			message = classified
		}
		if owner != nil {
			owner.Close()
		}
		if err := cleanupTemp(e.manager.paths.Temp, temp); err != nil && log != nil {
			log.write("cleanup", "app", "error", err.Error(), nil)
		}
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
		if log != nil {
			if jobErr != nil {
				log.write("finish", "app", "error", jobErr.Error(), nil)
			}
			log.close()
		}
		e.finish(jobErr, message)
	}()
	log, jobErr = newLog(e.manager.paths.Logs, id)
	if jobErr != nil {
		return
	}
	log.write("preparing", "app", "identity", fmt.Sprintf("logical_job_id=%s attempt=%d execution_id=%s", e.job.view.ID, e.job.view.Attempt, id), nil)
	log.httpContext = r.HTTPContext
	log.browserContext = r.Browser
	log.sourceURL = r.URL
	log.write("preparing", "app", "request", r.URL, nil)
	log.write("preparing", "app", "http_context", fmt.Sprintf("http_profile=%s headers=%s", r.httpProfile, strings.Join(r.HTTPContext.headerNames(), ",")), nil)
	if e.manager.preflight != nil {
		e.phase("preparing", "Проверка…")
		jobErr = e.manager.preflight(e.ctx)
		if jobErr != nil {
			message = "Не удалось подготовить Mangar. Проверьте подключение к интернету и нажмите «Повторить»."
			return
		}
	}
	paths := map[string]string{}
	for attempt := 0; attempt < 2; attempt++ {
		for _, t := range []tools.Tool{tools.YtDlp, tools.FFmpeg, tools.Deno} {
			var acquired map[string]string
			var release func()
			acquired, release, jobErr = e.manager.acquire(e.ctx, t)
			if jobErr != nil {
				message = "Не удалось подготовить компоненты. Попробуйте повторить загрузку."
				break
			}
			releases = append(releases, release)
			for exe, p := range acquired {
				paths[exe] = p
				log.write("lease", string(t), "path", p, nil)
			}
		}
		if jobErr == nil {
			break
		}
		if attempt != 0 || e.manager.preflight == nil {
			return
		}
		for _, release := range releases {
			release()
		}
		releases = nil
		clear(paths)
		// A runtime may disappear after this boot was verified. Acquire invalidates
		// the tools cache; re-enter the SAME shared repair gate, retaining the job.
		if jobErr = e.manager.preflight(e.ctx); jobErr != nil {
			return
		}
	}
	for _, exe := range []string{"yt-dlp.exe", "ffmpeg.exe", "ffprobe.exe", "deno.exe"} {
		if !filepath.IsAbs(paths[exe]) {
			jobErr = errors.New("Некорректный managed lease")
			message = "Не удалось подготовить компоненты. Попробуйте повторить загрузку."
			return
		}
	}
	if filepath.Dir(paths["ffmpeg.exe"]) != filepath.Dir(paths["ffprobe.exe"]) {
		jobErr = errors.New("FFmpeg pair mismatch")
		return
	}
	if jobErr = safeTemp(e.manager.paths.Temp, temp); jobErr != nil {
		return
	}
	if jobErr = os.MkdirAll(temp, 0700); jobErr != nil {
		return
	}
	owner, jobErr = tempOwner(temp)
	if jobErr != nil {
		return
	}
	var cookieJar string
	if r.Browser != nil {
		log.write("preparing", "app", "browser_capture", fmt.Sprintf("browser_capture=true media_type=%s cookies=%d exact_cookies=%t", r.Browser.MediaType, len(r.Browser.Cookies), r.Browser.ExactCookies), nil)
		if len(r.Browser.Cookies) > 0 {
			cookieJar, jobErr = writeCookieJar(temp, r.Browser)
			if jobErr != nil {
				return
			}
		}
	}
	e.phase("fetching_info", "Получение информации…")
	var meta metadata
	var input string
	var lastProgress time.Time
	args := ytdlpArgs(paths, temp, r.URL, r.HTTPContext)
	if cookieJar != "" {
		args = append([]string{"--cookies", cookieJar}, args...)
	}
	// Header values never enter logs, including argv. Raw output is separately redacted.
	log.write("fetching_info", "yt-dlp", "argv", fmt.Sprint(ytdlpArgs(paths, temp, r.URL)), nil)
	var failureMessage string
	eligibleCookies, forbidden, cookieAttempt := false, false, false
	drm := false
	onLine := func(stream, line string) {
		if stream == "stderr" {
			drm = drm || strings.Contains(strings.ToLower(line), "drm")
			eligibleCookies = eligibleCookies || cookieFallbackEligible(line)
			if match := httpErrorPattern.FindStringSubmatch(line); len(match) > 1 && match[1] == "403" {
				forbidden = true
			}
			if cookieAttempt {
				if classified := cookieAccessFailure(line); classified != "" {
					failureMessage = classified
				}
			}
		}
		logLine := line
		if strings.HasPrefix(line, metaPrefix) {
			var safeMeta map[string]json.RawMessage
			if json.Unmarshal([]byte(strings.TrimPrefix(line, metaPrefix)), &safeMeta) == nil && safeMeta != nil {
				safeMeta["id"] = json.RawMessage(`"[redacted]"`)
				var title string
				json.Unmarshal(safeMeta["title"], &title)
				if sensitiveName(title) || sourceSecretTitle(title, r.URL) {
					safeMeta["title"] = json.RawMessage(`"[redacted]"`)
				}
				data, _ := json.Marshal(safeMeta)
				logLine = metaPrefix + string(data)
			} else {
				logLine = metaPrefix + "[redacted]"
			}
		}
		log.write(e.snapshot().State, "yt-dlp", stream, logLine, nil)
		if stream == "stderr" {
			if classified := browserFailure(line, r); classified != "" && !strings.Contains(failureMessage, "cookies браузера") {
				failureMessage = classified
				if classified == networkFailure {
					e.change(func(s *Job) {
						s.Message = "Сервер не отвечает. Повторяем подключение…"
					})
				}
			}
		}
		switch {
		case strings.HasPrefix(line, metaPrefix):
			var next metadata
			if json.Unmarshal([]byte(strings.TrimPrefix(line, metaPrefix)), &next) == nil && (next.ID != "" || next.Title != "") {
				meta = next
				e.change(func(s *Job) { s.Title, _ = browserTitle(next.Title, r) })
			}
		case strings.HasPrefix(line, pathPrefix):
			json.Unmarshal([]byte(strings.TrimPrefix(line, pathPrefix)), &input)
		case strings.HasPrefix(line, postPrefix):
			e.phase("postprocessing", "Объединение…")
		default:
			p, percent, ok := parseProgress(line)
			if ok {
				now := e.manager.now()
				emit := now.Sub(lastProgress) >= 200*time.Millisecond || e.snapshot().State != "downloading"
				if emit {
					lastProgress = now
				}
				e.progress(p, percent, now, emit)
			}
		}
	}
	code, err := e.manager.run(e.ctx, paths["yt-dlp.exe"], args, temp, onLine)
	if err != nil && !drm && r.Browser == nil && eligibleCookies && r.cookieMode != "off" && e.ctx.Err() == nil {
		browser := r.cookieMode
		if browser == "auto" {
			browser = e.manager.detectBrowser()
		}
		if browser == "chrome" || browser == "edge" || browser == "firefox" {
			log.exit(e.snapshot().State, paths["yt-dlp.exe"], code, err)
			log.write("fetching_info", "app", "browser_cookies", "Одна попытка cookies браузера: "+browser, nil)
			meta, input, failureMessage = metadata{}, "", ""
			forbidden = false
			cookieAttempt = true
			log.browserCookies = true
			args = append([]string{"--cookies-from-browser", browser}, args...)
			code, err = e.manager.run(e.ctx, paths["yt-dlp.exe"], args, temp, onLine)
		} else {
			failureMessage = "Cookies браузера недоступны: выберите Chrome, Edge, Firefox или используйте расширение"
		}
	}
	if err != nil && !drm && forbidden && (r.Browser != nil || (cookieAttempt && !strings.Contains(failureMessage, "cookies браузера"))) && e.ctx.Err() == nil {
		target := availableBrowserTarget(e.ctx, e.manager.run, paths["yt-dlp.exe"], temp, r.HTTPContext)
		if target != "" {
			log.exit(e.snapshot().State, paths["yt-dlp.exe"], code, err)
			log.write("fetching_info", "yt-dlp", "impersonation", "Одна повторная попытка с доступным browser target="+target, nil)
			meta = metadata{}
			input = ""
			failureMessage = ""
			code, err = e.manager.run(e.ctx, paths["yt-dlp.exe"], append([]string{"--impersonate", target}, args...), temp, onLine)
		}
	}
	log.exit(e.snapshot().State, paths["yt-dlp.exe"], code, err)
	if err != nil {
		jobErr = err
		message = "Ошибка загрузки"
		if meta.ID == "" {
			message = "Не удалось получить информацию о видео"
		}
		if failureMessage != "" {
			message = failureMessage
		}
		return
	}
	if input == "" || safeTemp(temp, input) != nil {
		jobErr = errors.New("Нет безопасного intermediate path")
		message = "Не удалось подготовить MP4"
		return
	}
	e.phase("finalizing", "Подготовка MP4…")
	var final string
	final, jobErr = finalize(e.ctx, e.manager.run, paths, input, temp, log)
	if jobErr != nil {
		message = "Не удалось подготовить MP4"
		return
	}
	var output string
	title, fallback := browserTitle(meta.Title, r)
	mediaID := meta.ID
	if fallback {
		mediaID = e.job.view.ID[:8]
	}
	if e.job.view.BatchIndex > 0 {
		output, jobErr = publishBatch(e.ctx, final, r.Directory, e.job.view.OutputFilename, id, e.commit)
	} else {
		output, jobErr = publish(e.ctx, final, r.Directory, title, mediaID, id, e.commit)
	}
	if jobErr != nil {
		message = "Не удалось сохранить MP4 в выбранную папку"
		return
	}
	e.change(func(s *Job) { s.FinalPath = output })
	log.write("completed", "app", "output", output, nil)
}
