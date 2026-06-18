//go:build !windows

package ort

import (
	"github.com/ebitengine/purego"
)

func loadLibrary(path string) (uintptr, error) {
	libHandle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil || libHandle == 0 {
		return 0, err
	}
	return libHandle, nil
}

func getSymbol(handle uintptr, symbol string) (uintptr, error) {
	return purego.Dlsym(handle, symbol)
}

func closeLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	return purego.Dlclose(handle)
}
