//go:build windows

package tokenizers

import (
	"os"

	"github.com/pkg/errors"

	"golang.org/x/sys/windows"
)

func loadLibrary(path string) (uintptr, error) {
	handle, err := windows.LoadLibrary(path)
	if err != nil || handle == 0 {
		return 0, errors.Wrapf(err, "failed to load shared library: %s", path)
	}
	return uintptr(handle), nil
}

func isLibraryValid(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	// Try to load the library to verify it's valid
	if handle, err := windows.LoadLibrary(path); err == nil {
		_ = windows.FreeLibrary(handle)
		return true
	}
	return false
}

func closeLibrary(handle uintptr) error {
	if handle == 0 {
		return errors.New("invalid library handle")
	}
	if err := windows.FreeLibrary(windows.Handle(handle)); err != nil {
		return errors.Errorf("failed to close library: %s", err.Error())
	}
	return nil
}

func symbolExists(handle uintptr, name string) error {
	_, err := windows.GetProcAddress(windows.Handle(handle), name)
	return err
}
