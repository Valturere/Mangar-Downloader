//go:build windows

package windowsplatform

import (
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/options/windows"
	win32 "golang.org/x/sys/windows"
)

func WebViewMessages() *windows.Messages {
	return &windows.Messages{
		InstallationRequired: "Для запуска требуется Microsoft WebView2 Runtime. Установите его и повторите запуск.",
		UpdateRequired:       "Обновите Microsoft WebView2 Runtime и повторите запуск.",
		MissingRequirements:  "Не хватает компонента",
		Webview2NotInstalled: "Microsoft WebView2 Runtime не установлен",
		Error:                "Ошибка запуска",
		FailedToInstall:      "Не удалось установить Microsoft WebView2 Runtime. Проверьте сеть и повторите установку.",
		DownloadPage:         "Установите Microsoft WebView2 Runtime с официального сайта. Минимальная версия: ",
		PressOKToInstall:     "Нажмите ОК для установки Microsoft WebView2 Runtime.",
		ContactAdmin:         "Установите или восстановите Microsoft WebView2 Runtime и повторите запуск. Официальный сайт: https://developer.microsoft.com/microsoft-edge/webview2/",
		InvalidFixedWebview2: "Не удалось запустить указанный Microsoft WebView2 Runtime. Проверьте его установку и версию.",
		WebView2ProcessCrash: "Процесс Microsoft WebView2 Runtime завершился с ошибкой. Перезапустите приложение; при повторении восстановите runtime.",
	}
}

// Ошибка до создания WebView2 должна быть видна и в Windows GUI subsystem.
func ShowStartupError(message string) {
	text, _ := win32.UTF16PtrFromString(message)
	title, _ := win32.UTF16PtrFromString("Mangar Downloader — ошибка запуска")
	win32.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
