//go:build windows

package chroma

import (
	"os"
	"runtime"

	"golang.org/x/sys/windows"
)

func loadLibrary(path string) (uintptr, error) {
	plan, err := resolveLibraryLoadPlan(path, runtime.GOOS, os.Getenv, os.Stat)
	if err != nil {
		return 0, err
	}

	loadAttempts := make([]string, 0, len(plan.candidates))
	for _, candidate := range plan.candidates {
		handle, loadErr := windows.LoadLibrary(candidate.path)
		if loadErr == nil {
			if handle != 0 {
				// Successful load returns only the handle; plan warnings are intentionally
				// surfaced only on error to preserve the current Init/loadLibrary API.
				return uintptr(handle), nil
			}

			loadAttempts = append(loadAttempts, formatLoadAttempt(candidate, nil))
			return 0, formatLibraryLoadError(plan, loadAttempts)
		}
		loadAttempts = append(loadAttempts, formatLoadAttempt(candidate, loadErr))
	}

	return 0, formatLibraryLoadError(plan, loadAttempts)
}
