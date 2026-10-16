//go:build windows

package tools

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SystemBootEnvironmentInformation (90), SYSTEM_BOOT_ENVIRONMENT_INFORMATION:
// https://github.com/winsiderss/phnt/blob/master/ntexapi.h
// Query the kernel's boot GUID directly: no wall clock estimate, WMI or shell.
// Sleep/hibernate retain the kernel boot; a new kernel boot changes the GUID.
func windowsBootIdentity() (string, error) {
	var info struct {
		BootIdentifier windows.GUID
		FirmwareType   uint32
		_              uint32
		BootFlags      uint64
	}
	var returned uint32
	err := windows.NtQuerySystemInformation(90, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), &returned)
	if err != nil {
		return "", fmt.Errorf("boot identity: %w", err)
	}
	if returned != uint32(unsafe.Sizeof(info)) || info.BootIdentifier == (windows.GUID{}) {
		return "", fmt.Errorf("invalid boot identity response")
	}
	return info.BootIdentifier.String(), nil
}
