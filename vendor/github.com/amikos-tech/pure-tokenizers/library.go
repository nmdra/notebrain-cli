//go:build !windows

package tokenizers

import (
	"os"

	"github.com/pkg/errors"

	"github.com/ebitengine/purego"
)

func loadLibrary(path string) (uintptr, error) {
	libHandle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil || libHandle == 0 {
		return 0, errors.Wrapf(err, "failed to load shared library: %s", path)
	}
	return libHandle, nil
}

// isLibraryValid checks if the library file exists and is valid
func isLibraryValid(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	// Try to load the library to verify it's valid
	if libh, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
		_ = purego.Dlclose(libh)
		return true
	}
	return false
}

func closeLibrary(handle uintptr) error {
	if handle == 0 {
		return errors.New("invalid library handle")
	}
	if err := purego.Dlclose(handle); err != nil {
		return errors.Errorf("failed to close library: %s", err.Error())
	}
	return nil
}

func symbolExists(handle uintptr, name string) error {
	_, err := purego.Dlsym(handle, name)
	return err
}
