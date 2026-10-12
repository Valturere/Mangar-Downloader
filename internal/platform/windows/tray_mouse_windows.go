//go:build windows && amd64

package windowsplatform

import (
	"errors"
	"sync"

	"golang.org/x/sys/windows"
)

const trayCallbackMessage = 0x401 // pinned systray 1.2.2: WM_USER+1
const trayLeftUp = 0x202

var trayMouse struct {
	sync.Mutex
	hwnd, previous uintptr
	open           func()
}

// One callback trampoline per process, never one per hide/show.
var trayMouseProc = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
	trayMouse.Lock()
	previous, open := trayMouse.previous, trayMouse.open
	owned := trayMouse.hwnd == hwnd && previous != 0
	trayMouse.Unlock()
	if !owned {
		result, _, _ := user32.NewProc("DefWindowProcW").Call(hwnd, msg, wp, lp)
		return result
	}
	if msg == trayCallbackMessage && lp == trayLeftUp {
		// Never wait for App.mu or Wails on the tray message loop.
		if open != nil {
			go open()
		}
		return 0
	}
	if msg == 0x10 || msg == 0x2 || msg == 0x82 { // WM_CLOSE / DESTROY / NCDESTROY
		// Restore on the owning message thread BEFORE forwarding destruction.
		trayMouse.Lock()
		restored, _, _ := setWindowProc.Call(hwnd, ^uintptr(3), previous)
		if restored != 0 {
			trayMouse.hwnd = 0
			trayMouse.previous = 0
			trayMouse.open = nil
		}
		trayMouse.Unlock()
	}
	result, _, _ := callWindowProc.Call(previous, hwnd, msg, wp, lp)
	return result
})

func installTrayMouse(open func()) error {
	trayLookupMu.Lock()
	trayHWND.Store(0)
	enumWindows.Call(findTrayWindow, 0)
	hwnd := trayHWND.Load()
	trayLookupMu.Unlock()
	if hwnd == 0 {
		return errors.New("Не удалось подключить ЛКМ трея; используйте меню трея")
	}
	trayMouse.Lock()
	defer trayMouse.Unlock()
	if trayMouse.previous != 0 {
		if trayMouse.hwnd == hwnd {
			return nil
		}
		return errors.New("Обработчик ЛКМ трея уже подключён")
	}
	previous, _, _ := setWindowProc.Call(hwnd, ^uintptr(3), trayMouseProc)
	if previous == 0 {
		return errors.New("Не удалось подключить ЛКМ трея; используйте меню трея")
	}
	trayMouse.hwnd, trayMouse.previous, trayMouse.open = hwnd, previous, open
	return nil
}
