//go:build windows

package ort

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// goStringToORTChar converts a Go string to ORTCHAR_T for Windows.
// The returned backing object must be kept alive by the caller until ORT
// has finished using the returned pointer (for example via runtime.KeepAlive
// immediately after the ORT call).
func goStringToORTChar(s string) (uintptr, any, error) {
	utf16, err := windows.UTF16FromString(s)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to convert path to UTF-16: %w", err)
	}
	// #nosec G103 -- Required for CGO-free FFI to pass wchar_t* path to ORT on Windows.
	return uintptr(unsafe.Pointer(unsafe.SliceData(utf16))), utf16, nil
}
