//go:build windows

package updates

import (
	"YTDLP-GUI-Mangar/internal/buildinfo"
	"YTDLP-GUI-Mangar/internal/installation"
	processrunner "YTDLP-GUI-Mangar/internal/process"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var ErrHelper = errors.New("Не удалось подготовить безопасную установку обновления. Повторите попытку")

type Plan struct {
	Schema         int      `json:"schema"`
	ProductID      string   `json:"product_id"`
	ParentPID      uint32   `json:"parent_pid"`
	ParentCreated  uint64   `json:"parent_created"`
	InstallDir     string   `json:"install_dir"`
	CurrentVersion string   `json:"current_version"`
	Manifest       Manifest `json:"manifest"`
}
type Result struct {
	Schema    int    `json:"schema"`
	ProductID string `json:"product_id"`
	Version   string `json:"version"`
	Code      string `json:"code"`
}

func processStamp(h windows.Handle) (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	e := windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), e
}
func processImage(h windows.Handle) (string, error) {
	b := make([]uint16, 32768)
	size := uint32(len(b))
	e := windows.QueryFullProcessImageName(h, 0, &b[0], &size)
	if e != nil {
		return "", e
	}
	return windows.UTF16ToString(b[:size]), nil
}
func defaultInstallDir() (string, error) {
	base, e := os.UserCacheDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(base, "Programs", "Mangar"), nil
}
func PreparePlan(dir, installDir, current string, m Manifest) (string, error) {
	root, e := updatesRoot()
	if e != nil || validateStage(dir, root) != nil {
		return "", ErrHelper
	}
	expected, e := defaultInstallDir()
	if e != nil || !strings.EqualFold(installDir, expected) {
		return "", ErrHelper
	}
	metadata, e := installation.ReadMetadata(installDir)
	if e != nil || metadata.Version != current || metadata.SourceDirty {
		return "", ErrHelper
	}
	source, e := os.Open(filepath.Join(installDir, "mangar-updater.exe"))
	if e != nil {
		return "", ErrHelper
	}
	defer source.Close()
	destination, e := os.OpenFile(filepath.Join(dir, "mangar-updater.exe"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", ErrHelper
	}
	sum := sha256.New()
	_, e = io.Copy(io.MultiWriter(destination, sum), source)
	if e == nil {
		e = destination.Sync()
	}
	e = errors.Join(e, destination.Close())
	if e != nil || !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), metadata.Files["mangar-updater.exe"]) {
		return "", ErrHelper
	}
	process, e := windows.GetCurrentProcess()
	if e != nil {
		return "", ErrHelper
	}
	stamp, e := processStamp(process)
	if e != nil {
		return "", ErrHelper
	}
	plan := Plan{1, installation.ProductID, uint32(os.Getpid()), stamp, installDir, current, m}
	data, e := json.Marshal(plan)
	if e != nil {
		return "", ErrHelper
	}
	file := filepath.Join(dir, "plan.json")
	if e = os.WriteFile(file, data, 0600); e != nil {
		return "", ErrHelper
	}
	return file, nil
}
func readPlan(file string) (Plan, string, error) {
	var plan Plan
	root, e := updatesRoot()
	if e != nil {
		return plan, "", ErrHelper
	}
	dir := filepath.Dir(file)
	if filepath.Base(file) != "plan.json" || validateStage(dir, root) != nil {
		return plan, "", ErrHelper
	}
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 16384 {
		return plan, "", ErrHelper
	}
	data, e := os.ReadFile(file)
	if e != nil || len(data) > 16384 || uniqueJSON(data) != nil {
		return plan, "", ErrHelper
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&plan) != nil {
		return plan, "", ErrHelper
	}
	expected, e := defaultInstallDir()
	if e != nil || !strings.EqualFold(plan.InstallDir, expected) || installation.SafeDirectory(plan.InstallDir) != nil || plan.Schema != 1 || plan.ProductID != installation.ProductID || plan.ParentPID == 0 || plan.ParentCreated == 0 {
		return plan, "", ErrHelper
	}
	current, e := buildinfo.Parse(plan.CurrentVersion)
	if e != nil {
		return plan, "", ErrHelper
	}
	latest, e := buildinfo.Parse(plan.Manifest.Version)
	if e != nil || latest.Compare(current) <= 0 {
		return plan, "", ErrHelper
	}
	descriptor, _ := json.Marshal(plan.Manifest)
	if _, e = ParseManifest(descriptor, plan.Manifest.Version); e != nil {
		return plan, "", ErrHelper
	}
	return plan, dir, nil
}
func lockInstaller(file string, m Manifest) (*os.File, error) {
	info, e := os.Lstat(file)
	if e != nil || !info.Mode().IsRegular() {
		return nil, ErrIntegrity
	}
	p, e := windows.UTF16PtrFromString(file)
	if e != nil {
		return nil, ErrHelper
	}
	h, e := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		return nil, ErrHelper
	}
	f := os.NewFile(uintptr(h), file)
	info, e = f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != m.InstallerSize {
		f.Close()
		return nil, ErrIntegrity
	}
	sum := sha256.New()
	_, e = io.Copy(sum, f)
	if e != nil || !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), m.InstallerSHA256) {
		f.Close()
		return nil, ErrIntegrity
	}
	return f, nil
}
func descendants(parent, helper uint32, seen map[uint32]windows.Handle) ([]uint32, error) {
	snapshot, e := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if e != nil {
		return nil, e
	}
	defer windows.CloseHandle(snapshot)
	all := map[uint32]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e = windows.Process32First(snapshot, &entry); e == nil; e = windows.Process32Next(snapshot, &entry) {
		all[entry.ProcessID] = entry.ParentProcessID
	}
	wanted := map[uint32]bool{parent: true}
	// Keep observed ancestors in the graph after their exit; a later snapshot
	// can otherwise lose a still-running grandchild whose parent already exited.
	for id := range seen {
		wanted[id] = true
	}
	var result []uint32
	for changed := true; changed; {
		changed = false
		for id, pid := range all {
			if id != helper && wanted[pid] && !wanted[id] {
				wanted[id] = true
				result = append(result, id)
				changed = true
			}
		}
	}
	return result, nil
}
func waitParentTree(ctx context.Context, parent windows.Handle, pid uint32) error {
	handles := map[uint32]windows.Handle{}
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()
	for {
		ids, e := descendants(pid, uint32(os.Getpid()), handles)
		if e != nil {
			return ErrHelper
		}
		for _, id := range ids {
			if handles[id] == 0 {
				h, e := windows.OpenProcess(windows.SYNCHRONIZE, false, id)
				if e == nil {
					handles[id] = h
				} else if e != windows.ERROR_INVALID_PARAMETER && e != windows.ERROR_NOT_FOUND {
					return ErrHelper
				}
			}
		}
		done, e := windows.WaitForSingleObject(parent, 0)
		if e != nil {
			return ErrHelper
		}
		allDone := done == windows.WAIT_OBJECT_0
		for _, h := range handles {
			state, e := windows.WaitForSingleObject(h, 0)
			if e != nil || state != windows.WAIT_OBJECT_0 {
				allDone = false
			}
		}
		if allDone {
			return nil
		}
		select {
		case <-ctx.Done():
			return ErrHelper
		case <-time.After(50 * time.Millisecond):
		}
	}
}
func writeResult(version, code string) {
	base, e := os.UserCacheDir()
	if e != nil {
		return
	}
	dir := filepath.Join(base, installation.ProductID, "data")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	data, _ := json.Marshal(Result{1, installation.ProductID, version, code})
	_ = os.WriteFile(filepath.Join(dir, "update-result.json"), data, 0600)
}

// Apply contains no HTTP or Credential Manager reads. NSIS alone replaces files.
func Apply(file string) error {
	plan, dir, e := readPlan(file)
	if e != nil {
		return ErrHelper
	}
	root, _ := updatesRoot()
	ready := filepath.Join(dir, "ready.json")
	fail := func(code string) error {
		_ = os.WriteFile(ready, []byte(`{"ready":false}`), 0600)
		writeResult(plan.Manifest.Version, code)
		return ErrHelper
	}
	lease, e := installation.UpdateLease(os.Getenv("LOCALAPPDATA"))
	if e != nil {
		return fail("busy")
	}
	held := true
	defer func() {
		if held {
			windows.CloseHandle(lease)
		}
	}()
	parent, e := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, plan.ParentPID)
	if e != nil {
		return fail("parent")
	}
	defer windows.CloseHandle(parent)
	stamp, e := processStamp(parent)
	if e != nil || stamp != plan.ParentCreated {
		return fail("parent")
	}
	image, e := processImage(parent)
	if e != nil || !strings.EqualFold(image, filepath.Join(plan.InstallDir, "YTDLP-GUI-Mangar.exe")) {
		return fail("parent")
	}
	metadata, e := installation.ReadMetadata(plan.InstallDir)
	if e != nil || metadata.Version != plan.CurrentVersion {
		return fail("version")
	}
	self, e := os.Executable()
	if e != nil || !strings.EqualFold(self, filepath.Join(dir, "mangar-updater.exe")) {
		return fail("helper")
	}
	copy, e := os.Open(self)
	if e != nil {
		return fail("helper")
	}
	sum := sha256.New()
	_, e = io.Copy(sum, copy)
	copy.Close()
	if e != nil || !strings.EqualFold(hex.EncodeToString(sum.Sum(nil)), metadata.Files["mangar-updater.exe"]) {
		return fail("helper")
	}
	installer := filepath.Join(dir, plan.Manifest.InstallerAsset)
	locked, e := lockInstaller(installer, plan.Manifest)
	if e != nil {
		return fail("integrity")
	}
	defer locked.Close()
	if e = os.WriteFile(ready, []byte(`{"ready":true}`), 0600); e != nil {
		return fail("staging")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	e = waitParentTree(ctx, parent, plan.ParentPID)
	cancel()
	if e != nil {
		return fail("parent-timeout")
	}
	installCtx, stop := context.WithTimeout(context.Background(), 15*time.Minute)
	defer stop()
	output, e := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if e != nil {
		return fail("installer")
	}
	defer output.Close()
	_, e = processrunner.RunInJob(installCtx, installer, []string{"/S"}, plan.InstallDir, output, output)
	locked.Close()
	gateContext, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	gateError := waitInstallGate(gateContext)
	gateCancel()
	if e != nil {
		writeResult(plan.Manifest.Version, "installer")
		// A failed transaction may have restored the prior version. Restart it
		// only after both integrity and runtime version are confirmed.
		if gateError == nil && installation.Verify(plan.InstallDir, plan.CurrentVersion) == nil {
			windows.CloseHandle(lease)
			held = false
			restartApplication(plan.InstallDir)
		}
		return ErrHelper
	}
	if gateError != nil {
		return fail("installer-timeout")
	}
	if e = installation.Verify(plan.InstallDir, plan.Manifest.Version); e != nil {
		writeResult(plan.Manifest.Version, "validation")
		return ErrHelper
	}
	writeResult(plan.Manifest.Version, "success")
	windows.CloseHandle(lease)
	held = false
	if e = restartApplication(plan.InstallDir); e != nil {
		writeResult(plan.Manifest.Version, "restart")
		return ErrHelper
	}
	_ = CleanupStage(dir, root)
	return nil
}

func waitInstallGate(ctx context.Context) error {
	for installation.InstallBusy(os.Getenv("LOCALAPPDATA")) {
		select {
		case <-ctx.Done():
			return ErrHelper
		case <-time.After(20 * time.Millisecond):
		}
	}
	return nil
}

func restartApplication(dir string) error {
	command := exec.Command(filepath.Join(dir, "YTDLP-GUI-Mangar.exe"))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if e := command.Start(); e != nil {
		return e
	}
	return command.Process.Release()
}

// LaunchPrepared does not close the app until a validated helper acknowledges
// its update lease and the parent handle. The caller owns idle-queue admission.
func LaunchPrepared(ctx context.Context, file string) error {
	_, dir, e := readPlan(file)
	if e != nil {
		return e
	}
	ready := filepath.Join(dir, "ready.json")
	if _, e = os.Lstat(ready); !os.IsNotExist(e) {
		return ErrHelper
	}
	command := exec.Command(filepath.Join(dir, "mangar-updater.exe"), "--apply", file)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if command.Start() != nil {
		return ErrHelper
	}
	acknowledged := false
	defer func() {
		if !acknowledged {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if b, e := os.ReadFile(ready); e == nil && len(b) < 128 {
			var state struct {
				Ready bool `json:"ready"`
			}
			if json.Unmarshal(b, &state) == nil {
				if !state.Ready {
					return ErrHelper
				}
				acknowledged = true
				_ = command.Process.Release()
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ErrHelper
		case <-timer.C:
			return ErrHelper
		case <-ticker.C:
		}
	}
}
