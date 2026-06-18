//go:build !windows

package ort

// goStringToORTChar converts a Go string to ORTCHAR_T for Unix platforms.
// The returned backing object must be kept alive by the caller until ORT
// has finished using the returned pointer.
func goStringToORTChar(s string) (uintptr, any, error) {
	bytes, ptr := GoToCstring(s)
	return ptr, bytes, nil
}
