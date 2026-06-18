//go:build windows

package ort

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func lockFile(file *os.File) error {
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	return windows.LockFileEx(handle, flags, 0, 1, 0, &overlapped)
}

func unlockFile(file *os.File) error {
	handle := windows.Handle(file.Fd())
	var overlapped windows.Overlapped
	return windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
}

func isLockWouldBlock(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}
