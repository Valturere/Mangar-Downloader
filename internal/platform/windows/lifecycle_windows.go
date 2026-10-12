//go:build windows && amd64

package windowsplatform

import (
	"encoding/binary"
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const ExitQuestion = "Есть незавершённые загрузки. Закрыть программу и отменить их?"

var (
	mainLookupMu   sync.Mutex
	mainHWND       uintptr
	findMainWindow = windows.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != windows.GetCurrentProcessId() {
			return 1
		}
		var name [128]uint16
		getClassName.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if windows.UTF16ToString(name[:]) != "YTDLPGUIMangarWindow" {
			return 1
		}
		mainHWND = hwnd
		return 0
	})
	setWindowProc      = user32.NewProc("SetWindowLongPtrW")
	callWindowProc     = user32.NewProc("CallWindowProcW")
	postWindowMessage  = user32.NewProc("PostMessageW")
	exitDialogMessage  = registerExitDialogMessage()
	nativeDialogMu     sync.Mutex
	nativeDialogResult chan dialogResult
	sessionMu          sync.Mutex
	sessionPrevious    uintptr
	sessionEnd         func()
	sessionProc        = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		sessionMu.Lock()
		previous, end := sessionPrevious, sessionEnd
		sessionMu.Unlock()
		if msg == exitDialogMessage && exitDialogMessage != 0 {
			nativeDialogMu.Lock()
			result := nativeDialogResult
			nativeDialogResult = nil
			nativeDialogMu.Unlock()
			if result != nil {
				approved, err := showExitDialog(hwnd)
				result <- dialogResult{approved, err}
			}
			return 0
		}
		switch msg {
		case 0x11: // WM_QUERYENDSESSION: never veto logoff/shutdown.
			return 1
		case 0x16: // WM_ENDSESSION: FALSE means shutdown was abandoned.
			if wp != 0 && end != nil {
				end()
			}
		}
		result, _, _ := callWindowProc.Call(previous, hwnd, msg, wp, lp)
		return result
	})
)

type dialogResult struct {
	approved bool
	err      error
}

func registerExitDialogMessage() uintptr {
	name, _ := windows.UTF16PtrFromString("YTDLPGUIMangar.ConfirmExit")
	message, _, _ := user32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(name)))
	return message
}

func applicationWindow() uintptr {
	mainLookupMu.Lock()
	defer mainLookupMu.Unlock()
	mainHWND = 0
	enumWindows.Call(findMainWindow, 0)
	return mainHWND
}

// HandleSessionEnd subclasses only our own Wails HWND using documented Win32
// APIs. No tray-library internals or application restart are involved.
func HandleSessionEnd(end func()) error {
	hwnd := applicationWindow()
	if hwnd == 0 {
		return errors.New("Окно для завершения Windows session не найдено")
	}
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if sessionPrevious != 0 {
		return nil
	}
	previous, _, err := setWindowProc.Call(hwnd, ^uintptr(3), sessionProc) // GWLP_WNDPROC = -4
	if previous == 0 {
		return errors.New("Не удалось подключить завершение Windows session: " + err.Error())
	}
	sessionPrevious, sessionEnd = previous, end
	return nil
}

// ConfirmExit uses Common Controls v6 from the production manifest. The native
// TaskDialog has Russian custom buttons, Cancel default, Escape and an owner.
// TASKDIALOGCONFIG / TASKDIALOG_BUTTON are explicitly pack(1) in commctrl.h.
func ConfirmExit() (bool, error) {
	owner := applicationWindow()
	sessionMu.Lock()
	hooked := sessionPrevious != 0
	sessionMu.Unlock()
	if owner == 0 || !hooked || exitDialogMessage == 0 {
		return false, errors.New("Окно подтверждения выхода недоступно")
	}
	result := make(chan dialogResult, 1)
	nativeDialogMu.Lock()
	if nativeDialogResult != nil {
		nativeDialogMu.Unlock()
		return false, errors.New("Подтверждение выхода уже открывается")
	}
	nativeDialogResult = result
	nativeDialogMu.Unlock()
	// FIFO PostMessage runs after earlier Wails Invoke callbacks. TaskDialog
	// enters its nested message loop on the owner's STA thread, like file dialogs.
	posted, _, _ := postWindowMessage.Call(owner, exitDialogMessage, 0, 0)
	if posted == 0 {
		nativeDialogMu.Lock()
		nativeDialogResult = nil
		nativeDialogMu.Unlock()
		return false, errors.New("Не удалось открыть подтверждение выхода")
	}
	response := <-result
	return response.approved, response.err
}

func showExitDialog(owner uintptr) (bool, error) {
	iconic, _, _ := user32.NewProc("IsIconic").Call(owner)
	command := uintptr(5) // SW_SHOW preserves maximized.
	if iconic != 0 {
		command = 9
	} // SW_RESTORE
	user32.NewProc("ShowWindow").Call(owner, command)
	user32.NewProc("SetForegroundWindow").Call(owner)
	title, _ := windows.UTF16PtrFromString("Mangar Downloader")
	text, _ := windows.UTF16PtrFromString(ExitQuestion)
	cancel, _ := windows.UTF16PtrFromString("Отмена")
	closeText, _ := windows.UTF16PtrFromString("Закрыть")
	var buttons [24]byte
	var config [160]byte
	// Packed pointer fields are invisible to Go's stack relocator. Pin buffers
	// before writing addresses, so all native pointers remain stable across calls.
	var pins runtime.Pinner
	pins.Pin(&buttons[0])
	pins.Pin(&config[0])
	pins.Pin(title)
	pins.Pin(text)
	pins.Pin(cancel)
	pins.Pin(closeText)
	defer pins.Unpin()
	binary.LittleEndian.PutUint32(buttons[0:], 2) // IDCANCEL
	binary.LittleEndian.PutUint64(buttons[4:], uint64(uintptr(unsafe.Pointer(cancel))))
	binary.LittleEndian.PutUint32(buttons[12:], 100)
	binary.LittleEndian.PutUint64(buttons[16:], uint64(uintptr(unsafe.Pointer(closeText))))
	binary.LittleEndian.PutUint32(config[0:], uint32(len(config)))
	binary.LittleEndian.PutUint64(config[4:], uint64(owner))
	binary.LittleEndian.PutUint32(config[20:], 0x0008|0x1000) // ALLOW_DIALOG_CANCELLATION | POSITION_RELATIVE_TO_WINDOW
	binary.LittleEndian.PutUint64(config[28:], uint64(uintptr(unsafe.Pointer(title))))
	binary.LittleEndian.PutUint64(config[52:], uint64(uintptr(unsafe.Pointer(text)))) // pszContent
	binary.LittleEndian.PutUint32(config[60:], 2)
	binary.LittleEndian.PutUint64(config[64:], uint64(uintptr(unsafe.Pointer(&buttons[0]))))
	binary.LittleEndian.PutUint32(config[72:], 2)
	var button int32
	proc := windows.NewLazySystemDLL("comctl32.dll").NewProc("TaskDialogIndirect")
	if err := proc.Find(); err != nil {
		return false, errors.New("Нативное подтверждение выхода недоступно")
	}
	result, _, _ := proc.Call(uintptr(unsafe.Pointer(&config[0])), uintptr(unsafe.Pointer(&button)), 0, 0)
	runtime.KeepAlive(title)
	runtime.KeepAlive(text)
	runtime.KeepAlive(cancel)
	runtime.KeepAlive(closeText)
	runtime.KeepAlive(buttons)
	if int32(result) < 0 {
		return false, errors.New("Не удалось показать подтверждение выхода")
	}
	return button == 100, nil
}
