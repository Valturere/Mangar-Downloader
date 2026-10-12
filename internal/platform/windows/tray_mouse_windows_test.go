//go:build windows && amd64

package windowsplatform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var testMenuHWND uintptr
var findTestMenu = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
	var pid uint32
	getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	var name [128]uint16
	getClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
	if pid == windows.GetCurrentProcessId() && windows.UTF16ToString(name[:]) == "#32768" {
		testMenuHWND = hwnd
		return 0
	}
	return 1
})

func TestTrayMouseNative(t *testing.T) {
	if os.Getenv("MANGAR_TRAY_MOUSE_SMOKE") != "1" {
		t.Skip("opt-in native notification area")
	}
	icon, err := os.ReadFile(filepath.Join("..", "..", "..", "build", "windows", "icon.ico"))
	if err != nil {
		t.Fatal(err)
	}
	opened := make(chan struct{}, 20)
	open := func() { opened <- struct{}{} }
	// Deliberately fail the optional enhancement; the library still owns its menu.
	tray, err := startTray(icon, open, func() {}, func(func()) error { return errors.New("controlled hook failure") })
	if err != nil || tray.MouseHookError() == nil {
		t.Fatal("optional failure aborted tray", err)
	}
	defer tray.Stop()
	wait := func(check func() bool) {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("native condition timeout")
	}
	menu := func() uintptr { testMenuHWND = 0; enumWindows.Call(findTestMenu, 0); return testMenuHWND }
	wait(tray.Available)
	hwnd := trayHWND.Load()
	postWindowMessage.Call(hwnd, trayCallbackMessage, 100, 0x205)
	wait(func() bool { return menu() != 0 })
	postWindowMessage.Call(hwnd, 0x1F, 0, 0) // WM_CANCELMODE dismisses nested menu.
	wait(func() bool { return menu() == 0 })
	if err = installTrayMouse(open); err != nil {
		t.Fatal(err)
	}
	trayMouse.Lock()
	previous := trayMouse.previous
	trayMouse.Unlock()
	if err = installTrayMouse(open); err != nil {
		t.Fatal("duplicate install", err)
	}
	trayMouse.Lock()
	unchanged := trayMouse.previous == previous
	trayMouse.Unlock()
	if !unchanged {
		t.Fatal("double hook")
	}
	for range 10 {
		postWindowMessage.Call(hwnd, trayCallbackMessage, 100, trayLeftUp)
		select {
		case <-opened:
		case <-time.After(3 * time.Second):
			t.Fatal("LMB did not open")
		}
		if menu() != 0 {
			t.Fatal("LMB showed menu")
		}
	}
	name, _ := windows.UTF16PtrFromString("TaskbarCreated")
	taskbar, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(name)))
	postWindowMessage.Call(hwnd, taskbar, 0, 0)
	postWindowMessage.Call(hwnd, trayCallbackMessage, 100, trayLeftUp)
	select {
	case <-opened:
	case <-time.After(3 * time.Second):
		t.Fatal("TaskbarCreated lost hook")
	}
	if err = tray.Stop(); err != nil {
		t.Fatal(err)
	}
	if err = tray.Stop(); err != nil {
		t.Fatal("Stop not idempotent", err)
	}
	trayMouse.Lock()
	clean := trayMouse.hwnd == 0 && trayMouse.previous == 0 && trayMouse.open == nil
	trayMouse.Unlock()
	if !clean || tray.Available() {
		t.Fatal("stale hook/icon after Stop")
	}
	t.Log("optional failure retains RMB menu; LMB x10 no menu; duplicate install; TaskbarCreated; Stop restore/clear/idempotent: pass")
}
