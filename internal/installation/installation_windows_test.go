//go:build windows

package installation

import (
	"YTDLP-GUI-Mangar/internal/browserbridge"
	"YTDLP-GUI-Mangar/internal/distribution"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Программа & (%) 日本語")
	if e := os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	m := Metadata{Schema: 1, ProductID: ProductID, Version: "0.9.0", Architecture: "win-x64", SourceCommit: "fixture", Files: map[string]string{}}
	for _, name := range PayloadNames {
		m.Files[name] = strings.Repeat("a", 64)
		_ = os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0600)
	}
	b, _ := json.Marshal(m)
	if e := os.WriteFile(filepath.Join(dir, "installation.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	return dir
}
func testNamespace(t *testing.T) string {
	var id [12]byte
	_, _ = rand.Read(id[:])
	prefix := fmt.Sprintf(`Software\YTDLPGUIMangar\Tests\Native-%x`, id)
	t.Cleanup(func() {
		for _, l := range nativeLocations(prefix) {
			k, e := registry.OpenKey(registry.CURRENT_USER, l.path, registry.SET_VALUE|l.view)
			if e == nil {
				_ = k.DeleteValue("")
				k.Close()
			}
			parentPath := l.path[:strings.LastIndex(l.path, `\`)]
			parent, e := registry.OpenKey(registry.CURRENT_USER, parentPath, registry.WRITE|l.view)
			if e == nil {
				_ = registry.DeleteKey(parent, "ru.mangar.browser")
				parent.Close()
			}
		}
	})
	return prefix
}
func TestNativeViewsIdentityMigrationAndOwnership(t *testing.T) {
	dir := fixture(t)
	prefix := testNamespace(t)
	if e := registerAt(dir, prefix); e != nil {
		t.Fatal(e)
	}
	for _, l := range nativeLocations(prefix) {
		v, e := readLocation(l)
		if e != nil || !v.exists || v.value != NativePath(dir) {
			t.Fatal("missing view", l.view, e)
		}
	}
	b, _ := os.ReadFile(NativePath(dir))
	var m NativeManifest
	_ = json.Unmarshal(b, &m)
	if len(m.AllowedOrigins) != 2 || m.AllowedOrigins[0] != distribution.Origin() || m.AllowedOrigins[1] != distribution.UnpackedOrigin || m.Path != filepath.Join(dir, "mangar-browser-host.exe") {
		t.Fatal("wrong production manifest")
	}
	foreign := nativeLocations(prefix)[3]
	if e := setLocation(foreign, filepath.Join(t.TempDir(), "foreign.json")); e != nil {
		t.Fatal(e)
	}
	before := foreign.value
	current, _ := readLocation(foreign)
	before = current.value
	if e := registerAt(dir, prefix); e == nil {
		t.Fatal("foreign overwritten")
	}
	if e := unregisterAt(dir, prefix); e != nil {
		t.Fatal(e)
	}
	current, _ = readLocation(foreign)
	if !current.exists || current.value != before {
		t.Fatal("foreign removed")
	}
	if e := removeOwnedLocation(foreign, before); e != nil {
		t.Fatal(e)
	}
	devPath := filepath.Join(t.TempDir(), "ru.mangar.browser.json")
	dev, _ := json.Marshal(NativeManifest{"ru.mangar.browser", "Mangar browser integration", `C:\dev\mangar-browser-host.exe`, "stdio", []string{developmentOrigin}})
	_ = os.WriteFile(devPath, dev, 0600)
	for _, l := range nativeLocations(prefix) {
		if e := setLocation(l, devPath); e != nil {
			t.Fatal(e)
		}
	}
	if e := registerAt(dir, prefix); e != nil {
		t.Fatal("known dev migration", e)
	}
	b, _ = os.ReadFile(NativePath(dir))
	if strings.Contains(string(b), `C:\\dev\\`) || strings.Contains(string(b), `.local`) {
		t.Fatal("dev helper path leaked into production")
	}
}
func TestPreflightReinstallDowngradeForeignAndReparse(t *testing.T) {
	dir := fixture(t)
	for _, version := range []string{"0.9.0", "1.0.0"} {
		if e := Preflight(dir, version); e != nil {
			t.Fatal(e)
		}
	}
	if e := Preflight(dir, "0.8.0"); e == nil {
		t.Fatal("downgrade accepted")
	}
	empty := t.TempDir()
	if e := Preflight(empty, "0.9.0"); e != nil {
		t.Fatal(e)
	}
	_ = os.WriteFile(filepath.Join(empty, "YTDLP-GUI-Mangar.exe"), []byte("foreign"), 0600)
	if e := Preflight(empty, "0.9.0"); e == nil {
		t.Fatal("unknown binary overwritten")
	}
	if e := SafeDirectory("relative"); e == nil {
		t.Fatal("relative accepted")
	}
	if e := SafeDirectory(`C:\` + strings.Repeat("a", 250)); e == nil {
		t.Fatal("NSIS unsafe long directory accepted")
	}
	link := filepath.Join(t.TempDir(), "linked")
	if e := os.Symlink(dir, link); e == nil {
		if e = SafeDirectory(link); e == nil {
			t.Fatal("reparse accepted")
		}
	}
}
func TestGuardParent(t *testing.T) {
	if os.Getenv("MANGAR_GUARD_PARENT") != "1" {
		return
	}
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
	os.Exit(0)
}
func TestGuardLaunchSerializationAndParentCleanup(t *testing.T) {
	scope := t.TempDir()
	ready := filepath.Join(t.TempDir(), "ready.ini")
	command := exec.Command(os.Args[0], "-test.run=^TestGuardParent$")
	command.Env = append(os.Environ(), "MANGAR_GUARD_PARENT=1")
	input, e := command.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = command.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = input.Close(); _ = command.Process.Kill() })
	done := make(chan error, 1)
	go func() { done <- Guard(uint32(command.Process.Pid), scope, ready) }()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if _, e = os.Stat(ready); e == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, e = os.Stat(ready); e != nil {
		t.Fatal("guard not ready")
	}
	if !Busy(scope) {
		t.Fatal("guard invisible")
	}
	if gate, e := BeginLaunch(scope); e == nil {
		windows.CloseHandle(gate)
		t.Fatal("startup admitted")
	}
	id, _ := browserbridge.UserIdentity(scope)
	app, first, e := browserbridge.Claim(id)
	if app != 0 {
		windows.CloseHandle(app)
	}
	if e != nil || !first {
		t.Fatal("guard claimed app identity")
	}
	_ = input.Close()
	_ = command.Wait()
	select {
	case e = <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("orphan guard")
	}
	if Busy(scope) {
		t.Fatal("stale guard")
	}
	gate, e := BeginLaunch(scope)
	if e != nil {
		t.Fatal(e)
	}
	windows.CloseHandle(gate)
}
func TestGuardRejectsLiveAppWithoutCancellingIt(t *testing.T) {
	scope := t.TempDir()
	id, _ := browserbridge.UserIdentity(scope)
	app, first, e := browserbridge.Claim(id)
	if e != nil || !first {
		t.Fatal(e)
	}
	defer windows.CloseHandle(app)
	ready := filepath.Join(t.TempDir(), "ready.ini")
	if e = Guard(uint32(os.Getpid()), scope, ready); e == nil {
		t.Fatal("live app accepted")
	}
	if _, e = os.Stat(ready); !os.IsNotExist(e) {
		t.Fatal("false ready")
	}
	if Busy(scope) {
		t.Fatal("leaked install mutex")
	}
}

func TestInterruptedJournalBlocksLaunchUntilInstallerRepair(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	dir := filepath.Join(base, "Programs", "Mangar")
	os.MkdirAll(dir, 0700)
	cases := []struct {
		text    string
		pending bool
	}{{"[transaction]\r\nowner=" + ProductID + "\r\nstate=pending\r\n", true}, {"[transaction]\r\nowner=" + ProductID + "\r\nstate=committed\r\n", false}, {"[transaction]\nowner=foreign\nstate=committed\n", true}, {"[transaction]\nowner=" + ProductID + "\nstate=committed-extra\n", true}, {"[transaction]\nowner=" + ProductID + "\nstate=committed\nstate=pending\n", true}}
	for _, fixture := range cases {
		for _, unicode := range []bool{false, true} {
			data := []byte(fixture.text)
			if unicode {
				encoded := windows.StringToUTF16(fixture.text)
				data = []byte{0xff, 0xfe}
				for _, r := range encoded[:len(encoded)-1] {
					data = append(data, byte(r), byte(r>>8))
				}
			}
			os.WriteFile(filepath.Join(dir, "transaction.ini"), data, 0600)
			if TransactionPending(dir) != fixture.pending {
				t.Fatal("wrong journal detection")
			}
			gate, e := BeginLaunch(base)
			if gate != 0 {
				windows.CloseHandle(gate)
			}
			if fixture.pending != (e != nil) {
				t.Fatal("unsafe app launch", e)
			}
		}
	}
}
