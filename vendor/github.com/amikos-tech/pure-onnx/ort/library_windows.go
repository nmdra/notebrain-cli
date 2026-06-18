//go:build windows

package ort

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

func loadLibrary(path string) (uintptr, error) {
	handle, err := windows.LoadLibrary(path)
	if err != nil || handle == 0 {
		return 0, err
	}
	return uintptr(handle), nil
}

func getSymbol(handle uintptr, symbol string) (uintptr, error) {
	proc, err := windows.GetProcAddress(windows.Handle(handle), symbol)
	if err != nil {
		return 0, err
	}
	return uintptr(unsafe.Pointer(proc)), nil
}

func closeLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	return windows.FreeLibrary(windows.Handle(handle))
}
