//go:build windows

package installation

import (
	"golang.org/x/sys/windows/registry"
	"strings"
)

func WebViewAvailable() bool {
	for _, hive := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY} {
			k, e := registry.OpenKey(hive, `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, registry.QUERY_VALUE|view)
			if e != nil {
				continue
			}
			v, _, e := k.GetStringValue("pv")
			k.Close()
			if e == nil && v != "" && strings.Trim(v, "0.") != "" {
				return true
			}
		}
	}
	return false
}
