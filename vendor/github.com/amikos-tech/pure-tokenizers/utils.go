package tokenizers

import (
	"math"
	"unsafe"
)

func MasksFromBuf(buf Buffer) (special, attention []uint32) {
	if buf.Len > uintptr(math.MaxInt) {
		return nil, nil
	}
	// #nosec G115 -- buf.Len is bounded to math.MaxInt above.
	n := int(buf.Len)
	if n == 0 {
		return nil, nil // Return empty slices if length is zero
	}

	if buf.SpecialTokensMask != nil {
		special = unsafe.Slice(buf.SpecialTokensMask, n) // #nosec G103 -- Slice view over Rust-owned FFI output buffer.

	}
	if buf.AttentionMask != nil {
		attention = unsafe.Slice(buf.AttentionMask, n) // #nosec G103 -- Slice view over Rust-owned FFI output buffer.
	}

	return
}

func TokensFromBuf(buf Buffer) []string {
	if buf.Tokens == nil || buf.Len == 0 {
		return nil
	}
	ptrs := unsafe.Slice(buf.Tokens, buf.Len) // #nosec G103 -- Token pointer array is returned from trusted Rust FFI.
	out := make([]string, 0, len(ptrs))

	for _, p := range ptrs {
		if p == nil {
			continue
		}
		q := unsafe.Pointer(p) // #nosec G103 -- Pointer points to FFI-managed null-terminated token bytes.
		var n uintptr
		for *(*byte)(unsafe.Add(q, n)) != 0 {
			n++
		}
		b := unsafe.Slice((*byte)(q), n) // #nosec G103 -- Converts token bytes from FFI pointer/length to Go slice.
		out = append(out, string(b))
	}
	return out
}
