//go:build windows

package installation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// Windows' built-in WScript adapter resolves the link without executing it.
// Script text is fixed; the validated installation path is passed as data.
func RemoveShortcuts(dir string) error {
	return shortcutOperation(dir, "remove")
}
func CheckShortcuts(dir string) error       { return shortcutOperation(dir, "check") }
func CheckDesktopShortcut(dir string) error { return shortcutOperation(dir, "check-desktop") }
func shortcutOperation(dir, action string) error {
	if SafeDirectory(dir) != nil {
		return errors.New("Неизвестная папка установки")
	}
	script := `$ErrorActionPreference='Stop'; $target=Join-Path $env:MANGAR_UNINSTALL_DIR 'YTDLP-GUI-Mangar.exe'; $shell=New-Object -ComObject WScript.Shell; $folders=@([Environment]::GetFolderPath('Programs')); if($env:MANGAR_SHORTCUT_ACTION -eq 'remove') { $folders+= [Environment]::GetFolderPath('DesktopDirectory') }; if($env:MANGAR_SHORTCUT_ACTION -eq 'check-desktop') { $folders=@([Environment]::GetFolderPath('DesktopDirectory')) }; foreach($folder in $folders) { $file=Join-Path $folder 'Mangar Downloader.lnk'; if(Test-Path -LiteralPath $file -PathType Leaf) { $link=$shell.CreateShortcut($file); $owned=[string]::Equals($link.TargetPath,$target,[StringComparison]::OrdinalIgnoreCase); if($env:MANGAR_SHORTCUT_ACTION.StartsWith('check')) { if(!$owned) { throw 'Foreign shortcut' } } elseif($owned) { Remove-Item -LiteralPath $file -Force } } }`
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", script)
	command.Env = append(os.Environ(), "MANGAR_UNINSTALL_DIR="+dir, "MANGAR_SHORTCUT_ACTION="+action)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if command.Run() != nil {
		return errors.New("Не удалось проверить ярлыки приложения")
	}
	return nil
}
