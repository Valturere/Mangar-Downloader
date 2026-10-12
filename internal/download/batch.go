package download

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxTXTBytes = 2 << 20
const MaxTXTPositions = 1000
const MaxTXTLineBytes = 16 << 10

type BatchEntry struct {
	Index   int
	URL     string
	Invalid bool
}
type BatchResult struct {
	BatchID  string   `json:"batchID"`
	Filename string   `json:"filename"`
	Total    int      `json:"total"`
	Queued   int      `json:"queued"`
	Invalid  int      `json:"invalid"`
	Snapshot Snapshot `json:"snapshot"`
}

// ParseTXT bounds both the total stream and individual lines, including blank lines.
// Invalid URLs consume a position; invalid UTF-8/binary input rejects the whole file.
func ParseTXT(reader io.Reader) ([]BatchEntry, error) {
	limited := &io.LimitedReader{R: reader, N: MaxTXTBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), MaxTXTLineBytes+2)
	entries := []BatchEntry{}
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			line = strings.TrimPrefix(line, "\ufeff")
			first = false
		}
		if len(line) > MaxTXTLineBytes {
			return nil, errors.New("Строка TXT превышает 16 КиБ")
		}
		if !utf8.ValidString(line) || strings.ContainsRune(line, 0) {
			return nil, errors.New("TXT должен быть текстовым файлом UTF-8")
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(entries) == MaxTXTPositions {
			return nil, errors.New("В TXT допускается не более 1000 непустых строк")
		}
		source, err := ValidateURL(line)
		entries = append(entries, BatchEntry{Index: len(entries) + 1, URL: source, Invalid: err != nil})
	}
	if limited.N == 0 {
		return nil, errors.New("Размер TXT превышает 2 МиБ")
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, errors.New("Строка TXT превышает 16 КиБ")
		}
		return nil, errors.New("Не удалось прочитать TXT. Проверьте доступ к файлу")
	}
	if len(entries) == 0 {
		return nil, errors.New("TXT не содержит непустых строк")
	}
	return entries, nil
}

func readTXT(path string) ([]BatchEntry, error) {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".txt") {
		return nil, errors.New("Выберите один файл с расширением .txt")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("Не удалось открыть TXT. Проверьте наличие файла и доступ к нему")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("Выберите обычный TXT-файл")
	}
	if info.Size() > MaxTXTBytes {
		return nil, errors.New("Размер TXT превышает 2 МиБ")
	}
	return ParseTXT(f)
}

func destination(directory string) (string, error) {
	if directory == "" {
		return "", errors.New("Выберите папку для загрузки.")
	}
	if !filepath.IsAbs(directory) {
		return "", errors.New("Нет доступа к выбранной папке")
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", errors.New("Нет доступа к выбранной папке")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("Нет доступа к выбранной папке")
	}
	return filepath.Clean(resolved), nil
}

func batchName(index int) string { return fmt.Sprintf("%d.mp4", index) }

// Disk preflight and in-memory reservation admission share the scheduler mutex.
// Publication still uses a no-overwrite atomic rename against external writers.
func (m *Manager) batchConflictsLocked(directory string, entries []BatchEntry, exclude *queuedJob) error {
	conflicts := []string{}
	for _, entry := range entries {
		if entry.Invalid {
			continue
		}
		name := batchName(entry.Index)
		_, err := os.Lstat(filepath.Join(directory, name))
		conflict := err == nil
		if err != nil && !os.IsNotExist(err) {
			return errors.New("Не удалось проверить файлы в выбранной папке")
		}
		for _, j := range m.jobs {
			if j != exclude && j.view.BatchIndex == entry.Index && !j.view.InvalidSource && strings.EqualFold(j.request.Directory, directory) {
				conflict = true
				break
			}
		}
		if conflict {
			conflicts = append(conflicts, name)
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	shown := strings.Join(conflicts[:min(5, len(conflicts))], ", ")
	if len(conflicts) > 5 {
		shown += fmt.Sprintf(" и ещё %d", len(conflicts)-5)
	}
	return fmt.Errorf("В выбранной папке уже существуют или зарезервированы файлы пакетной загрузки: %s", shown)
}

func (m *Manager) ImportTXT(path, directory string) (BatchResult, error) {
	entries, err := readTXT(path)
	if err != nil {
		return BatchResult{}, err
	}
	directory, err = destination(directory)
	if err != nil {
		return BatchResult{}, err
	}
	batchID, err := newID()
	if err != nil {
		return BatchResult{}, errors.New("Не удалось создать пакет")
	}
	result := BatchResult{BatchID: batchID, Filename: filepath.Base(path), Total: len(entries)}
	jobs := make([]*queuedJob, 0, len(entries))
	cookieMode := m.admissionCookieMode()
	for _, entry := range entries {
		id, err := newID()
		if err != nil {
			return BatchResult{}, errors.New("Не удалось создать пакет")
		}
		view := Job{ID: id, ExecutionID: id, Attempt: 1, BatchID: batchID, BatchIndex: entry.Index, BatchSource: result.Filename, OutputFilename: batchName(entry.Index), InvalidSource: entry.Invalid, State: "queued", Message: "В очереди"}
		if entry.Invalid {
			view.State = "failed"
			view.Message = "Некорректная ссылка"
			view.Error = view.Message
			result.Invalid++
		} else {
			view.Source = safeSource(entry.URL)
			result.Queued++
		}
		request, err := prepareHTTPRequest(Request{URL: entry.URL, Directory: directory, cookieMode: cookieMode})
		if err != nil {
			return BatchResult{}, err
		}
		jobs = append(jobs, &queuedJob{request: request, view: view})
	}
	m.mu.Lock()
	if m.updatePaused {
		m.mu.Unlock()
		return BatchResult{}, errors.New("Приложение готовится к обновлению")
	}
	if m.closing {
		m.mu.Unlock()
		return BatchResult{}, errors.New("Приложение завершает работу")
	}
	if err = m.batchConflictsLocked(directory, entries, nil); err != nil {
		m.mu.Unlock()
		return BatchResult{}, err
	}
	for _, j := range jobs {
		m.sequence++
		j.view.Sequence = m.sequence
		j.view.CreatedAt = m.now()
		if j.view.InvalidSource {
			now := m.now()
			j.view.FinishedAt = &now
		} else {
			m.pending = append(m.pending, j)
		}
		m.jobs = append(m.jobs, j)
	}
	m.revision++
	m.scheduleLocked()
	result.Snapshot = m.snapshotLocked()
	m.mu.Unlock()
	m.notify(result.Snapshot)
	return result, nil
}
