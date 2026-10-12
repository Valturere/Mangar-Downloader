//go:build windows

package windowsplatform

import (
	"os"
	"sync"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	trayLookupMu sync.Mutex
	trayHWND     atomic.Uintptr
	user32       = windows.NewLazySystemDLL("user32.dll")
	getWindowPID = user32.NewProc("GetWindowThreadProcessId")
	getClassName = user32.NewProc("GetClassNameW")
	enumWindows  = user32.NewProc("EnumWindows")
	getIconRect  = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconGetRect")
	// Единственный callback на процесс: NewCallback не освобождает trampoline.
	findTrayWindow = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != uint32(os.Getpid()) {
			return 1
		}
		var name [128]uint16
		getClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if windows.UTF16ToString(name[:]) != "SystrayClass" {
			return 1
		}
		trayHWND.Store(hwnd)
		return 0
	})
)

type notifyIconIdentifier struct {
	Size   uint32
	Window uintptr
	ID     uint32
	GUID   windows.GUID
}

func (t *Tray) Available() bool {
	select {
	case <-t.done:
		return false
	default:
	}
	trayLookupMu.Lock()
	defer trayLookupMu.Unlock()
	trayHWND.Store(0)
	enumWindows.Call(findTrayWindow, 0)
	hwnd := trayHWND.Load()
	if hwnd == 0 {
		return false
	}
	// ID 100 принадлежит закреплённой версии getlantern/systray 1.2.2.
	id := notifyIconIdentifier{Window: hwnd, ID: 100}
	id.Size = uint32(unsafe.Sizeof(id))
	var rect [4]int32
	status, _, _ := getIconRect.Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&rect[0])))
	return status == 0
}
