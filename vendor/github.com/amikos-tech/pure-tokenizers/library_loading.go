package tokenizers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

// LoadTokenizerLibrary loads the tokenizer shared library from the specified path
// or attempts to find it through various fallback mechanisms:
// 1. User-provided path
// 2. TOKENIZERS_LIB_PATH environment variable
// 3. Cached library in platform-specific directory
// 4. Automatic download from releases.amikos.tech (with GitHub Releases fallback)
func LoadTokenizerLibrary(userPath string) (uintptr, error) {
	// Priority 1: User-provided path
	if userPath != "" {
		if _, err := os.Stat(userPath); err == nil { // #nosec G703 -- userPath is an explicit caller-supplied library override.
			libh, err := loadLibrary(userPath)
			if err != nil {
				return 0, errors.Wrapf(err, "failed to load library from user-provided path: %s", userPath)
			}
			if !isLibraryABIVerified(userPath) {
				if err := verifyLibraryABICompatibilityHandle(libh); err != nil {
					if closeErr := closeLibrary(libh); closeErr != nil {
						err = fmt.Errorf("%w; additionally failed to close library handle: %v", err, closeErr)
					}
					return 0, errors.Wrapf(err, "library at user-provided path is ABI/symbol incompatible: %s", userPath)
				}
				markLibraryABIVerified(userPath)
			}
			return libh, nil
		}
		return 0, errors.Errorf("library file not found at user-provided path: %s", userPath)
	}

	// Priority 2: Environment variable
	if envPath := os.Getenv("TOKENIZERS_LIB_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil { // #nosec G703 -- TOKENIZERS_LIB_PATH is an intentional user-controlled override.
			libh, err := loadLibrary(envPath)
			if err != nil {
				return 0, errors.Wrapf(err, "failed to load library from TOKENIZERS_LIB_PATH: %s", envPath)
			}
			if !isLibraryABIVerified(envPath) {
				if err := verifyLibraryABICompatibilityHandle(libh); err != nil {
					if closeErr := closeLibrary(libh); closeErr != nil {
						err = fmt.Errorf("%w; additionally failed to close library handle: %v", err, closeErr)
					}
					return 0, errors.Wrapf(err, "library at TOKENIZERS_LIB_PATH is ABI/symbol incompatible: %s", envPath)
				}
				markLibraryABIVerified(envPath)
			}
			return libh, nil
		}
		return 0, errors.Errorf("library file not found at TOKENIZERS_LIB_PATH: %s", envPath)
	}

	// Priority 3: Cached library
	cachedPath := GetCachedLibraryPath()
	var cachedLoadErr error
	if _, statErr := os.Stat(cachedPath); statErr == nil {
		shouldClearCache := false

		libh, err := loadLibrary(cachedPath)
		if err != nil {
			cachedLoadErr = errors.Wrapf(err, "failed to load cached library from %s", cachedPath)
			shouldClearCache = true
		} else {
			if !isLibraryABIVerified(cachedPath) {
				if err := verifyLibraryABICompatibilityHandle(libh); err != nil {
					if closeErr := closeLibrary(libh); closeErr != nil {
						err = fmt.Errorf("%w; additionally failed to close cached library handle: %v", err, closeErr)
					}
					cachedLoadErr = errors.Wrapf(err, "cached library failed ABI/symbol compatibility check: %s", cachedPath)
					shouldClearCache = true
				} else {
					markLibraryABIVerified(cachedPath)
				}
			}
			if cachedLoadErr == nil {
				return libh, nil
			}
		}

		// If cached library fails compatibility or load, clear cache once and re-download.
		if shouldClearCache {
			if clearErr := ClearLibraryCache(); clearErr != nil {
				_, _ = fmt.Fprintf(
					os.Stderr,
					"warning: failed to clear cached library %s (%v); continuing with re-download attempt\n",
					cachedPath,
					clearErr,
				)
			}
		}
	}

	// Priority 4: Download from releases endpoint (with GitHub fallback)
	if err := DownloadLibraryFromGitHub(cachedPath); err != nil {
		if cachedLoadErr != nil {
			return 0, errors.Wrapf(err, "failed to download library after cached load error: %v", cachedLoadErr)
		}
		return 0, errors.Wrap(err, "failed to download library from release endpoint")
	}

	// Try loading the newly downloaded library
	libh, err := loadLibrary(cachedPath)
	if err != nil {
		if cachedLoadErr != nil {
			return 0, errors.Wrapf(err, "failed to load downloaded library from %s (previous cached load error: %v)", cachedPath, cachedLoadErr)
		}
		return 0, errors.Wrapf(err, "failed to load downloaded library from: %s", cachedPath)
	}

	if !isLibraryABIVerified(cachedPath) {
		if err := verifyLibraryABICompatibilityHandle(libh); err != nil {
			if closeErr := closeLibrary(libh); closeErr != nil {
				err = fmt.Errorf("%w; additionally failed to close downloaded library handle: %v", err, closeErr)
			}
			if clearErr := ClearLibraryCache(); clearErr != nil {
				_, _ = fmt.Fprintf(
					os.Stderr,
					"warning: failed to clear incompatible downloaded library %s (%v)\n",
					cachedPath,
					clearErr,
				)
			}
			if cachedLoadErr != nil {
				return 0, errors.Wrapf(
					err,
					"downloaded library from %s failed ABI/symbol compatibility check (previous cached load error: %v)",
					cachedPath,
					cachedLoadErr,
				)
			}
			return 0, errors.Wrapf(err, "downloaded library from %s failed ABI/symbol compatibility check", cachedPath)
		}
		markLibraryABIVerified(cachedPath)
	}

	return libh, nil
}

// getLibraryName returns the platform-specific library name
func getLibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libtokenizers.dylib"
	case "linux":
		return "libtokenizers.so"
	case "windows":
		return "tokenizers.dll"
	default:
		return fmt.Sprintf("libtokenizers_%s", runtime.GOOS)
	}
}

// getCacheDir returns the platform-specific cache directory
func getCacheDir() string {
	var cacheDir string

	switch runtime.GOOS {
	case "darwin":
		// macOS: ~/Library/Caches/tokenizers/lib
		if home, err := os.UserHomeDir(); err == nil {
			cacheDir = filepath.Join(home, "Library", "Caches", "tokenizers", "lib")
		}
	case "linux":
		// Linux: Use XDG_CACHE_HOME or ~/.cache
		if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
			cacheDir = filepath.Join(xdgCache, "tokenizers", "lib")
		} else if home, err := os.UserHomeDir(); err == nil {
			cacheDir = filepath.Join(home, ".cache", "tokenizers", "lib")
		}
	case "windows":
		// Windows: %APPDATA%/tokenizers/lib
		if appData := os.Getenv("APPDATA"); appData != "" {
			cacheDir = filepath.Join(appData, "tokenizers", "lib")
		}
	}

	// Fallback to temp directory
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "tokenizers", "lib")
	}

	return cacheDir
}

// isMusl checks if the current Linux system uses musl libc
func isMusl() bool {
	// Check if ldd exists and mentions musl
	// This is a simplified check; a more robust implementation might
	// check the actual libc being linked
	if _, err := os.Stat("/lib/ld-musl-x86_64.so.1"); err == nil {
		return true
	}
	if _, err := os.Stat("/lib/ld-musl-aarch64.so.1"); err == nil {
		return true
	}
	return false
}
