//go:build windows

// Package credentials provides generic secret storage in Win32 Credential Manager.
package credentials

import (
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"unsafe"
)

var ErrMissing = errors.New("Учётные данные отсутствуют")
var ErrUnavailable = errors.New("Windows Credential Manager недоступен")
var dll = windows.NewLazySystemDLL("advapi32.dll")

type nativeCredential struct {
	Flags, Type             uint32
	TargetName, Comment     *uint16
	LastWritten             windows.Filetime
	BlobSize                uint32
	Blob                    *byte
	Persist, AttributeCount uint32
	Attributes              unsafe.Pointer
	Alias, UserName         *uint16
}
type Store struct{ target string }

func New(target string) *Store { return &Store{target: target} }
func (s *Store) Save(secret string) error {
	if len(secret) < 1 || len(secret) > 4096 {
		return errors.New("Недопустимый размер секрета")
	}
	name, err := windows.UTF16PtrFromString(s.target)
	if err != nil {
		return ErrUnavailable
	}
	blob := []byte(secret)
	defer clear(blob)
	c := nativeCredential{Type: 1, TargetName: name, BlobSize: uint32(len(blob)), Blob: &blob[0], Persist: 2}
	ok, _, _ := dll.NewProc("CredWriteW").Call(uintptr(unsafe.Pointer(&c)), 0)
	runtime.KeepAlive(c)
	runtime.KeepAlive(blob)
	if ok == 0 {
		return ErrUnavailable
	}
	return nil
}

// Read is backend-only; App never binds it to Wails.
func (s *Store) Read() (string, error) {
	name, err := windows.UTF16PtrFromString(s.target)
	if err != nil {
		return "", ErrUnavailable
	}
	var pointer *nativeCredential
	ok, _, callErr := dll.NewProc("CredReadW").Call(uintptr(unsafe.Pointer(name)), 1, 0, uintptr(unsafe.Pointer(&pointer)))
	if ok == 0 {
		if callErr == windows.ERROR_NOT_FOUND {
			return "", ErrMissing
		}
		return "", ErrUnavailable
	}
	defer dll.NewProc("CredFree").Call(uintptr(unsafe.Pointer(pointer)))
	if pointer == nil || pointer.Blob == nil || pointer.BlobSize < 1 || pointer.BlobSize > 4096 {
		return "", ErrUnavailable
	}
	return string(unsafe.Slice(pointer.Blob, int(pointer.BlobSize))), nil
}
func (s *Store) Remove() error {
	name, err := windows.UTF16PtrFromString(s.target)
	if err != nil {
		return ErrUnavailable
	}
	ok, _, callErr := dll.NewProc("CredDeleteW").Call(uintptr(unsafe.Pointer(name)), 1, 0)
	if ok == 0 && callErr != windows.ERROR_NOT_FOUND {
		return ErrUnavailable
	}
	return nil
}
