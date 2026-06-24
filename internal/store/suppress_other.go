//go:build !linux && !darwin

package store

// suppressOutputs is a no-op on non-Unix platforms.
func suppressOutputs(fn func()) {
	fn()
}
