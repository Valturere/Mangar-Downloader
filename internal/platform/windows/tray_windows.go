//go:build windows

package windowsplatform

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/getlantern/systray"
)

// Tray использует один lifecycle библиотеки на процесс.
// В одном процессе библиотека поддерживает только один жизненный цикл.
type Tray struct {
	done      chan struct{}
	stop      sync.Once
	hookError error
}

func StartTray(icon []byte, openWindow, exit func()) (*Tray, error) {
	return startTray(icon, openWindow, exit, installTrayMouse)
}

func startTray(icon []byte, openWindow, exit func(), installMouse func(func()) error) (*Tray, error) {
	t := &Tray{done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		// Окно tray и его GetMessage обязаны принадлежать одному OS-потоку.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(t.done)
		systray.Run(func() {
			systray.SetIcon(icon)
			systray.SetTooltip("Mangar Downloader")
			open := systray.AddMenuItem("Открыть Mangar", "Показать окно")
			systray.AddSeparator()
			quit := systray.AddMenuItem("Выйти", "Завершить приложение")
			t.hookError = installMouse(openWindow)
			close(ready)
			go func() {
				for {
					select {
					case <-open.ClickedCh:
						openWindow()
					case <-quit.ClickedCh:
						// Confirmation must not block Open or subsequent Exit after Cancel.
						go exit()
					case <-t.done:
						return
					}
				}
			}()
		}, nil)
	}()
	select {
	case <-ready:
		return t, nil
	case <-t.done:
		return nil, errors.New("Не удалось запустить системный трей")
	case <-time.After(5 * time.Second):
		systray.Quit()
		return nil, errors.New("Системный трей не ответил; окно останется доступным")
	}
}

// Failure of the optional mouse enhancement never disables the existing menu.
func (t *Tray) MouseHookError() error { return t.hookError }

func (t *Tray) Stop() error {
	t.stop.Do(func() {
		select {
		case <-t.done:
		default:
			systray.Quit()
		}
	})
	select {
	case <-t.done:
		return nil
	case <-time.After(3 * time.Second):
		return errors.New("Не удалось дождаться завершения системного трея")
	}
}
