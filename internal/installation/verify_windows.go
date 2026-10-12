//go:build windows

package installation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Verify(dir, version string) error {
	if e := SafeDirectory(dir); e != nil {
		return e
	}
	m, e := ReadMetadata(dir)
	if e != nil || m.Version != version {
		return errors.New("Версия установки не подтверждена")
	}
	for _, name := range PayloadNames {
		file, e := os.Open(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		sum := sha256.New()
		_, e = io.Copy(sum, file)
		file.Close()
		if e != nil || !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), m.Files[name]) {
			return errors.New("Установленные файлы повреждены")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(dir, "YTDLP-GUI-Mangar.exe"), "--version")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, e := command.Output()
	if e != nil || strings.TrimSpace(string(output)) != version {
		return errors.New("Runtime version не подтверждена")
	}
	return nil
}
