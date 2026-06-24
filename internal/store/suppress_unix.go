//go:build linux || darwin

package store

import (
	"os"

	"golang.org/x/sys/unix"
)

// suppressOutputs redirects both stdout and stderr of both Go and C-level
// standard streams to /dev/null for the duration of fn, then restores them.
// Used to silence the HNSW integrity check stdout prints from C++ code.
func suppressOutputs(fn func()) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		fn()
		return
	}
	defer func() { _ = devNull.Close() }()

	origStderr := os.Stderr
	origStdout := os.Stdout
	os.Stderr = devNull
	os.Stdout = devNull

	// Silence C-library side via dup2 for stdout (1) and stderr (2)
	savedFd1, err1 := unix.Dup(1)
	savedFd2, err2 := unix.Dup(2)

	if err1 == nil {
		_ = unix.Dup2(int(devNull.Fd()), 1)
		defer func() {
			_ = unix.Dup2(savedFd1, 1)
			_ = unix.Close(savedFd1)
		}()
	}
	if err2 == nil {
		_ = unix.Dup2(int(devNull.Fd()), 2)
		defer func() {
			_ = unix.Dup2(savedFd2, 2)
			_ = unix.Close(savedFd2)
		}()
	}

	defer func() {
		os.Stderr = origStderr
		os.Stdout = origStdout
	}()

	fn()
}
