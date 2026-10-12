//go:build windows

package installation

import (
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestUninstallRegistryForeignAndInterruptedOwnership(t *testing.T) {
	dir := t.TempDir()
	prefix := testNamespace(t)
	path := prefix + `\Uninstall`
	key, _, e := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS|registry.WOW64_64KEY)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for _, name := range []string{"DisplayName", "ProductID", "InstallLocation"} {
			key.DeleteValue(name)
		}
		key.Close()
		registry.DeleteKey(registry.CURRENT_USER, path)
	})
	key.SetStringValue("DisplayName", "Foreign")
	if checkUninstallRegistration(dir, path) == nil {
		t.Fatal("foreign key without ProductID accepted")
	}
	key.SetStringValue("ProductID", ProductID)
	key.SetStringValue("InstallLocation", filepath.Dir(dir))
	if checkUninstallRegistration(dir, path) == nil {
		t.Fatal("foreign install path accepted")
	}
	key.SetStringValue("InstallLocation", dir)
	if e = checkUninstallRegistration(dir, path); e != nil {
		t.Fatal(e)
	}
	key.DeleteValue("InstallLocation")
	if checkUninstallRegistration(dir, path) == nil {
		t.Fatal("incomplete key adopted without journal")
	}
	os.WriteFile(filepath.Join(dir, "transaction.ini"), []byte("[transaction]\nowner="+ProductID+"\nstate=pending\n"), 0600)
	if e = checkUninstallRegistration(dir, path); e != nil {
		t.Fatal("owned interrupted registry not recoverable", e)
	}
	key.SetStringValue("ProductID", "Foreign")
	if checkUninstallRegistration(dir, path) == nil {
		t.Fatal("foreign ProductID adopted through journal")
	}
}
