package ort

import "unsafe"

// CstringToGo converts a C null-terminated string pointer to a Go string.
// The pointer must point to a valid null-terminated string in memory.
// Returns empty string if ptr is 0 (null) or if the pointer appears invalid.
//
// Safety: This function uses byte-by-byte pointer arithmetic to avoid segfaults
// from creating slices that might cross memory page boundaries into unmapped memory.
// This is the safest approach for reading C-allocated strings with unknown length.
//
// The function includes defensive checks to detect corrupted or malicious pointers:
// - Rejects null pointers (ptr == 0)
// - Rejects low addresses (ptr < 4096) which are typically reserved/unmapped
// - Limits maximum string length to 1MB to prevent infinite loops
func CstringToGo(ptr uintptr) string {
	if ptr == 0 {
		return ""
	}

	// Sanity check: reject pointers to low addresses that are typically reserved
	// or unmapped on most operating systems (the first page, 0-4095).
	// This catches null-ish pointers, corrupted pointers, or potential attacks.
	// Valid C strings from proper libraries will always be well above this range.
	const minValidAddress = 4096
	if ptr < minValidAddress {
		return ""
	}

	// Find the length by reading byte-by-byte using pointer arithmetic.
	// This is safe because we only dereference one byte at a time,
	// and we trust that the C API provides valid null-terminated strings.
	// We don't create slices until we know the exact length.
	var length int
	const maxStringLen = 1 << 20 // 1MB safety limit
	for length < maxStringLen {
		// Read one byte at a time - safe even near page boundaries
		// #nosec G103 -- Necessary for CGO-free C string reading, pointer is validated above
		b := *(*byte)(unsafe.Pointer(ptr + uintptr(length)))
		if b == 0 {
			break // Found null terminator
		}
		length++
	}

	// Check if we hit the limit (likely invalid pointer or corrupted memory)
	if length >= maxStringLen {
		return ""
	}

	// Special case: empty string
	if length == 0 {
		return ""
	}

	// Now that we know the exact length, create a slice and convert to string
	// This is safe because we verified all bytes exist and found the null terminator
	// #nosec G103 -- Required for CGO-free FFI, length verified and null terminator found
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), length)
	return string(bytes)
}

// GoToCstring converts a Go string to a null-terminated byte slice suitable for passing to C functions.
// Returns the byte slice (which must be kept alive by the caller to prevent GC) and a uintptr to its first byte.
//
// IMPORTANT: The caller MUST keep the returned []byte alive for as long as the C function might access it.
// Example usage:
//
//	logIDBytes, logIDPtr := GoToCstring("my-log-id")
//	status := cFunction(logIDPtr)  // logIDBytes must stay in scope here
//	runtime.KeepAlive(logIDBytes)  // Ensure bytes aren't collected during C call
//
// Safety: Uses unsafe.SliceData (Go 1.20+) to safely extract the pointer to the slice's
// backing array. This is safer than &b[0] because SliceData is specifically designed for
// FFI use cases and prevents GC race conditions where the slice could be moved between
// taking the pointer and using it.
func GoToCstring(s string) ([]byte, uintptr) {
	// Create a null-terminated byte slice
	b := append([]byte(s), 0)

	// Return both the slice (to keep it alive) and pointer to first byte.
	// Use unsafe.SliceData instead of &b[0] for safer FFI pointer extraction.
	// SliceData returns a pointer to the underlying array, which is safe even
	// if the slice header itself is copied, as the backing array won't move.
	// #nosec G103 -- Required for CGO-free FFI to pass Go strings to C functions
	return b, uintptr(unsafe.Pointer(unsafe.SliceData(b)))
}
