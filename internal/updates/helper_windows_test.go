//go:build windows

package updates

import (
	"YTDLP-GUI-Mangar/internal/installation"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func planFixture(t *testing.T) (string, string, Manifest) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "Профиль & (%) 日本語"))
	install, _ := defaultInstallDir()
	os.MkdirAll(install, 0700)
	payload := []byte("MZ fixture helper not executed")
	sum := sha256.Sum256(payload)
	metadata := installation.Metadata{Schema: 1, ProductID: installation.ProductID, Version: "0.9.0", Architecture: "win-x64", SourceCommit: strings.Repeat("a", 40), Files: map[string]string{}}
	for _, name := range installation.PayloadNames {
		metadata.Files[name] = hex.EncodeToString(sum[:])
		os.WriteFile(filepath.Join(install, name), payload, 0600)
	}
	data, _ := json.Marshal(metadata)
	os.WriteFile(filepath.Join(install, "installation.json"), data, 0600)
	root, _ := updatesRoot()
	dir, e := NewStage(root)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = CleanupStage(dir, root) })
	return dir, install, validManifest([]byte("verified installer"))
}
func TestPlanIdentityPathVersionAndNoSecret(t *testing.T) {
	dir, install, m := planFixture(t)
	file, e := PreparePlan(dir, install, "0.9.0", m)
	if e != nil {
		t.Fatal(e)
	}
	plan, _, e := readPlan(file)
	if e != nil || plan.ParentPID != uint32(os.Getpid()) || plan.ParentCreated == 0 {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(file)
	if strings.Contains(string(data), fixturePrivateBody) || strings.Contains(string(data), "Authorization") {
		t.Fatal("secret in helper argv plan")
	}
	for _, mutate := range []func(*Plan){func(p *Plan) { p.Manifest.Version = "0.8.0" }, func(p *Plan) { p.ParentPID = 0 }, func(p *Plan) { p.InstallDir = filepath.Dir(install) }, func(p *Plan) { p.Manifest.InstallerAsset = "../installer.exe" }, func(p *Plan) { p.Manifest.Architecture = "win-arm64" }, func(p *Plan) { p.Schema = 2 }} {
		changed := plan
		mutate(&changed)
		b, _ := json.Marshal(changed)
		os.WriteFile(file, b, 0600)
		if _, _, e = readPlan(file); e == nil {
			t.Fatal("unsafe plan accepted")
		}
	}
}
func TestInstallerHashAndWriteDeleteLock(t *testing.T) {
	dir, _, m := planFixture(t)
	file := filepath.Join(dir, m.InstallerAsset)
	os.WriteFile(file, []byte("verified installer"), 0600)
	held, e := lockInstaller(file, m)
	if e != nil {
		t.Fatal(e)
	}
	if f, e := os.OpenFile(file, os.O_WRONLY|os.O_TRUNC, 0600); e == nil {
		f.Close()
		held.Close()
		t.Fatal("verified installer writable while held")
	}
	if os.Remove(file) == nil {
		held.Close()
		t.Fatal("verified installer deletable while held")
	}
	held.Close()
	os.WriteFile(file, []byte("modified installer"), 0600)
	if f, e := lockInstaller(file, m); e == nil {
		f.Close()
		t.Fatal("modified installer accepted")
	}
}
func TestUpdateLeaseDoesNotOwnAppMutex(t *testing.T) {
	scope := t.TempDir()
	lease, e := installation.UpdateLease(scope)
	if e != nil {
		t.Fatal(e)
	}
	if !installation.Busy(strings.ToUpper(scope)) {
		windows.CloseHandle(lease)
		t.Fatal("Windows scope case bypasses update gate")
	}
	if gate, e := installation.BeginLaunch(scope); e == nil {
		windows.CloseHandle(gate)
		windows.CloseHandle(lease)
		t.Fatal("startup during update")
	}
	if _, e := installation.UpdateLease(scope); e == nil {
		windows.CloseHandle(lease)
		t.Fatal("concurrent update")
	}
	windows.CloseHandle(lease)
	if installation.Busy(scope) {
		t.Fatal("update mutex leaked")
	}
}

func TestApplyRejectsForeignParentWithoutInstallerLaunch(t *testing.T) {
	dir, install, m := planFixture(t)
	file, e := PreparePlan(dir, install, "0.9.0", m)
	if e != nil {
		t.Fatal(e)
	}
	// This test binary is not the expected installed application image.
	if Apply(file) == nil {
		t.Fatal("foreign parent accepted")
	}
	if installation.Busy(os.Getenv("LOCALAPPDATA")) {
		t.Fatal("failed helper leaked update lease")
	}
	data, e := os.ReadFile(filepath.Join(dir, "ready.json"))
	if e != nil || string(data) != `{"ready":false}` {
		t.Fatal("bad helper acknowledgement", e)
	}
	if _, e = os.Stat(filepath.Join(dir, m.InstallerAsset)); !os.IsNotExist(e) {
		t.Fatal("installer touched")
	}
}
