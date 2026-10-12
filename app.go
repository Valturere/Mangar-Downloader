package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"YTDLP-GUI-Mangar/internal/apppaths"
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/download"
	windowsplatform "YTDLP-GUI-Mangar/internal/platform/windows"
	"YTDLP-GUI-Mangar/internal/settings"
	"YTDLP-GUI-Mangar/internal/tools"
	"YTDLP-GUI-Mangar/internal/updates"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
)

// App struct
type App struct {
	ctx              context.Context
	mu               sync.Mutex
	tray             trayController
	trayError        error
	dialogMu         sync.Mutex
	directoryDialog  func(context.Context, runtime.OpenDialogOptions) (string, error)
	fileDialog       func(context.Context, runtime.OpenDialogOptions) (string, error)
	toolManager      *tools.Manager
	toolContext      context.Context
	toolCancel       context.CancelFunc
	toolWG           sync.WaitGroup
	toolClosing      bool
	toolError        string
	toolPending      bool
	toolDOMReady     bool
	toolErrorEvent   func(string)
	downloadManager  *download.Manager
	exitPending      bool
	exitReady        bool
	hidden           bool
	shutdownOnce     sync.Once
	quitOnce         sync.Once
	shutdownDone     chan struct{}
	lifecycleCancel  context.CancelFunc
	showWindow       func()
	hideWindow       func()
	quit             func()
	confirmExit      func() (bool, error)
	settings         *settings.Store
	browserReady     chan struct{}
	browserReadyOnce sync.Once
	browserServer    *browserbridge.Server
	browserCaptures  map[string]*browserbridge.Capture
	browserPages     map[string]string
	browserOrder     []string
	browserDirectory string
	browserHandoff   sync.Mutex
	updateController *updates.Controller
	updateInstalling bool
	updateError      string
	updateRevision   uint64
}

// NewApp creates a new App application struct
func NewApp() *App {
	a := &App{directoryDialog: runtime.OpenDirectoryDialog, fileDialog: runtime.OpenFileDialog, shutdownDone: make(chan struct{}), browserReady: make(chan struct{}), browserCaptures: map[string]*browserbridge.Capture{}, browserPages: map[string]string{}}
	a.showWindow = func() { runtime.WindowShow(a.ctx) }
	a.hideWindow = func() { runtime.WindowHide(a.ctx) }
	a.quit = func() { runtime.Quit(a.ctx) }
	a.confirmExit = windowsplatform.ConfirmExit
	return a
}

// ChooseDirectory выбирает только путь, без сохранения настроек и создания заданий.
func (a *App) ChooseDirectory() (string, error) {
	return a.choosePath(a.directoryDialog, runtime.OpenDialogOptions{Title: "Выберите папку для загрузок"}, "Не удалось открыть выбор папки. Попробуйте ещё раз")
}

// ChooseTXT returns one native path; core ImportTXT owns parsing and admission.
func (a *App) ChooseTXT() (string, error) {
	path, err := a.choosePath(a.fileDialog, runtime.OpenDialogOptions{
		Title:   "Выберите список ссылок",
		Filters: []runtime.FileFilter{{DisplayName: "Текстовые файлы (*.txt)", Pattern: "*.txt"}},
	}, "Не удалось открыть выбор TXT. Попробуйте ещё раз")
	if err == nil && path != "" && !strings.EqualFold(filepath.Ext(path), ".txt") {
		return "", errors.New("Выберите файл с расширением .txt")
	}
	return path, err
}

func (a *App) choosePath(dialog func(context.Context, runtime.OpenDialogOptions) (string, error), options runtime.OpenDialogOptions, message string) (string, error) {
	a.mu.Lock()
	ctx, closing := a.ctx, a.toolClosing || a.exitPending
	a.mu.Unlock()
	if closing {
		return "", errors.New("Приложение завершает работу")
	}
	if ctx == nil {
		return "", errors.New("Окно приложения ещё не готово")
	}
	if !a.dialogMu.TryLock() {
		return "", errors.New("Сначала закройте открытый диалог выбора")
	}
	defer a.dialogMu.Unlock()
	path, err := dialog(ctx, options)
	if err != nil {
		// Сырой текст ошибки может содержать приватный путь: не передаём его в UI/лог.
		return "", errors.New(message)
	}
	return path, nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing {
		return
	}
	a.ctx = ctx
	paths, err := apppaths.Default()
	if err != nil {
		a.toolError = "Не удалось определить папку компонентов"
		return
	}
	logs, temps := download.CleanupStale(paths, time.Now())
	if logs+temps > 0 {
		tools.ToolLogger(paths.Logs)(tools.Tool("app"), "cleanup", fmt.Sprintf("Удалены устаревшие данные: logs=%d temp=%d", logs, temps))
	}
	a.toolContext, a.toolCancel = context.WithCancel(context.Background())
	a.settings, err = settings.Load(paths.Data)
	if err != nil {
		tools.ToolLogger(paths.Logs)(tools.Tool("app"), "settings", err.Error())
	}
	a.toolManager = tools.New(tools.Options{Paths: paths, Log: tools.ToolLogger(paths.Logs), OnChange: func(s tools.Snapshot) {
		a.mu.Lock()
		ready := a.toolDOMReady && !a.toolClosing
		a.mu.Unlock()
		// Startup detection may finish before WebView2 has a runtime. Its latest
		// snapshot is still available to the frontend when it mounts.
		if ready {
			runtime.EventsEmit(ctx, "tools:changed", s)
		}
	}})
	store := a.settings
	executable, _ := os.Executable()
	a.updateController = updates.NewController(updates.ControllerOptions{Version: buildinfo.Current(), Data: paths.Data, InstallDir: filepath.Dir(executable), StagingRoot: filepath.Join(paths.Root, "updates"), OnChange: func(s updates.Snapshot) {
		a.mu.Lock()
		ready := a.toolDOMReady && !a.toolClosing
		a.mu.Unlock()
		if ready {
			runtime.EventsEmit(ctx, "updates:changed", a.UpdateSnapshot())
		}
	}})
	a.downloadManager = download.New(download.Options{Paths: paths, Tools: a.toolManager, CookieMode: func() string { return store.Snapshot().BrowserCookieMode }, OnChange: func(s download.Snapshot) {
		a.mu.Lock()
		ready := a.toolDOMReady && !a.toolClosing
		a.mu.Unlock()
		if ready {
			runtime.EventsEmit(ctx, "download:changed", s)
		}
	}})
	a.toolWG.Add(1)
	a.toolPending = true
	go func() {
		defer a.toolWG.Done()
		err := a.toolManager.Detect(a.toolContext)
		a.mu.Lock()
		a.toolPending = false
		if err != nil {
			a.toolError = tools.UserError(err)
		}
		a.mu.Unlock()
		if err != nil {
			a.emitToolError(tools.UserError(err))
		}
	}()
}

func (a *App) DownloadSnapshot() download.Snapshot {
	a.mu.Lock()
	manager := a.downloadManager
	a.mu.Unlock()
	if manager == nil {
		return download.Snapshot{Jobs: []download.Job{}}
	}
	return manager.Snapshot()
}

func (a *App) StartDownload(request download.Request) (download.Snapshot, error) {
	// The WebView cannot inject native secret context.
	request.Browser = nil
	if request.HTTPContext != nil {
		request.HTTPContext.Captured = false
	}
	a.mu.Lock()
	if a.toolClosing || a.downloadManager == nil {
		a.mu.Unlock()
		return download.Snapshot{}, errors.New("Загрузка недоступна. Перезапустите приложение")
	}
	manager := a.downloadManager
	a.mu.Unlock()
	return manager.Start(request)
}

func (a *App) ImportTXT(path, directory string) (download.BatchResult, error) {
	a.mu.Lock()
	manager := a.downloadManager
	closing := a.toolClosing
	a.mu.Unlock()
	if closing || manager == nil {
		return download.BatchResult{}, errors.New("Импорт недоступен. Перезапустите приложение")
	}
	return manager.ImportTXT(path, directory)
}

func (a *App) DownloadAction(id, action string) (download.Snapshot, error) {
	a.mu.Lock()
	manager := a.downloadManager
	closing := a.toolClosing
	a.mu.Unlock()
	if closing || manager == nil {
		return download.Snapshot{}, errors.New("Приложение завершает работу")
	}
	switch action {
	case "cancel":
		return manager.Cancel(id)
	case "retry":
		return manager.Retry(id)
	case "remove":
		return manager.Remove(id)
	case "open", "folder":
		path, err := manager.CompletedPath(id)
		if err != nil {
			return download.Snapshot{}, err
		}
		if err = windowsplatform.OpenDownloaded(path, action == "folder"); err != nil {
			return download.Snapshot{}, err
		}
		return manager.Snapshot(), nil
	default:
		return download.Snapshot{}, errors.New("Неизвестное действие задания")
	}
}

func (a *App) OpenLogsFolder() error {
	a.mu.Lock()
	closing := a.toolClosing || a.exitPending
	ready := a.ctx != nil
	a.mu.Unlock()
	if closing {
		return errors.New("Приложение завершает работу")
	}
	if !ready {
		return errors.New("Окно приложения ещё не готово")
	}
	paths, err := apppaths.Default()
	if err != nil {
		return errors.New("Не удалось определить папку логов")
	}
	return windowsplatform.OpenLogs(paths.Logs)
}

// ToolsSnapshot returns DTOs only; frontend never assembles managed paths.
func (a *App) ToolsSnapshot() tools.Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolManager == nil {
		return tools.Snapshot{Tools: []tools.Status{}, Error: a.toolError}
	}
	s := a.toolManager.Snapshot()
	s.Error = a.toolError
	return s
}

// ComponentsAction is explicit network access and returns promptly to Wails.
func (a *App) ComponentsAction(action string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.updateInstalling || a.toolManager == nil {
		return errors.New("Компоненты недоступны. Перезапустите приложение")
	}
	if a.toolManager.Snapshot().Busy {
		return errors.New("Операция с компонентами уже выполняется")
	}
	if action != "install" && action != "check" {
		return errors.New("Неизвестное действие с компонентами")
	}
	// Adapter gate covers the interval before the manager goroutine enters begin.
	if a.toolPending {
		return errors.New("Операция с компонентами уже выполняется")
	}
	a.toolPending = true
	a.toolError = ""
	a.toolWG.Add(1)
	go func() {
		defer a.toolWG.Done()
		var err error
		if action == "check" {
			err = a.toolManager.CheckUpdates(a.toolContext)
		} else {
			err = a.toolManager.InstallNeeded(a.toolContext)
		}
		a.mu.Lock()
		a.toolPending = false
		if err != nil {
			a.toolError = tools.UserError(err)
		}
		a.mu.Unlock()
		if err != nil {
			a.emitToolError(tools.UserError(err))
		}
	}()
	return nil
}

func (a *App) emitToolError(message string) {
	if a.toolErrorEvent != nil {
		a.toolErrorEvent(message)
		return
	}
	a.mu.Lock()
	ready := a.toolDOMReady && !a.toolClosing
	a.mu.Unlock()
	if ready {
		runtime.EventsEmit(a.ctx, "tools:error", message)
	}
}

func (a *App) domReady(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing {
		return
	}
	a.toolDOMReady = true
	if a.updateController != nil && a.updateController.Snapshot().InstallSupported {
		go a.updateController.AutoStart()
	}
	if a.browserReady != nil {
		a.browserReadyOnce.Do(func() { close(a.browserReady) })
	}
	if a.tray != nil || a.trayError != nil {
		return
	}
	if err := windowsplatform.HandleSessionEnd(a.systemExit); err != nil {
		runtime.LogError(ctx, err.Error())
	}
	tray, err := windowsplatform.StartTray(trayIcon, a.openWindow, func() { a.requestExit(true) })
	a.tray, a.trayError = tray, err
	if tray != nil && tray.MouseHookError() != nil {
		runtime.LogError(ctx, tray.MouseHookError().Error())
	}
	lifecycleContext, cancel := context.WithCancel(context.Background())
	a.lifecycleCancel = cancel
	go a.watchTray(lifecycleContext)
}

func (a *App) SettingsSnapshot() settings.Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.settings == nil {
		return settings.Default()
	}
	return a.settings.Snapshot()
}

func (a *App) SetCloseToTray(value bool) (settings.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending {
		return settings.Default(), errors.New("Приложение завершает работу")
	}
	if a.settings == nil {
		return settings.Default(), errors.New("Настройки ещё не готовы. Повторите позже")
	}
	return a.settings.SetCloseToTray(value)
}
func (a *App) SetBrowserCookieMode(value string) (settings.Snapshot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending {
		return settings.Default(), errors.New("Приложение завершает работу")
	}
	if a.settings == nil {
		return settings.Default(), errors.New("Настройки ещё не готовы. Повторите позже")
	}
	return a.settings.SetBrowserCookieMode(value)
}

// HideForTray retains the diagnostic host's explicit hide operation.
func (a *App) HideForTray() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.toolClosing || a.exitPending {
		return errors.New("Приложение завершает работу")
	}
	if a.trayError != nil {
		return a.trayError
	}
	if a.tray == nil {
		return errors.New("Системный трей ещё не готов")
	}
	if !a.tray.Available() {
		return errors.New("Системный трей недоступен. Окно останется открытым")
	}
	a.hideWindow()
	a.hidden = true
	return nil
}
