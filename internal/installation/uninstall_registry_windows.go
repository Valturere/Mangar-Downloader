//go:build windows

package installation

import (
	"errors"
	"golang.org/x/sys/windows/registry"
	"strings"
)

func CheckUninstallRegistration(dir string) error {
	return checkUninstallRegistration(dir, UninstallKey)
}
func checkUninstallRegistration(dir, key string) error {
	k, e := registry.OpenKey(registry.CURRENT_USER, key, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if e == registry.ErrNotExist {
		return nil
	}
	if e != nil {
		return errors.New("Registry установки недоступен")
	}
	defer k.Close()
	owner, _, ownerError := k.GetStringValue("ProductID")
	location, _, locationError := k.GetStringValue("InstallLocation")
	if ownerError == nil && locationError == nil && owner == ProductID && strings.EqualFold(location, dir) {
		return nil
	}
	state := transactionState(dir)
	// An owned journal can repair the two non-atomic registry writes if one
	// marker exists; an unrelated existing key is never adopted.
	if state == "pending" || state == "committed" {
		if owner == ProductID && locationError == registry.ErrNotExist {
			return nil
		}
		if ownerError == registry.ErrNotExist && locationError == nil && strings.EqualFold(location, dir) {
			return nil
		}
	}
	return errors.New("Обнаружена чужая регистрация приложения")
}
