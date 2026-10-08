package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Keep this version in sync with chroma-go's defaultLocalClientConfig.
const defaultRuntimeVersion = "v0.3.4"
const runtimeDownloadEnv = "NOTEBRAIN_ALLOW_RUNTIME_DOWNLOAD"
const runtimeOSWindows = "windows"

// ErrRuntimeUnavailable identifies a local Chroma runtime that is not ready.
var ErrRuntimeUnavailable = errors.New("chroma runtime is unavailable")

// RuntimeAvailabilityError reports the runtime path and how to repair it.
type RuntimeAvailabilityError struct {
	Path     string
	Explicit bool
	Detail   string
}

func (e *RuntimeAvailabilityError) Error() string {
	if e == nil {
		return ErrRuntimeUnavailable.Error()
	}
	message := fmt.Sprintf("%s; %s", ErrRuntimeUnavailable, e.Detail)
	if e.Path != "" {
		message += fmt.Sprintf("; expected a readable, non-empty runtime file at %q", e.Path)
	}
	if e.Explicit {
		return message + "; correct CHROMA_LIB_PATH or unset it to use the default cache; runtime download opt-in does not override CHROMA_LIB_PATH"
	}
	if e.Path == "" {
		return message + "; set CHROMA_LIB_PATH to an installed Chroma runtime for this platform"
	}
	return message + "; to install the runtime explicitly, run `NOTEBRAIN_ALLOW_RUNTIME_DOWNLOAD=1 notebrain stats --format=json` (requires network access); if a previous download left " + fmt.Sprintf("%q", filepath.Join(filepath.Dir(e.Path), ".download.lock")) + ", wait for any active download; remove a stale lock only after confirming no download is active"
}

func (e *RuntimeAvailabilityError) Unwrap() error { return ErrRuntimeUnavailable }

// runtimeLibraryOptions checks availability without initializing Chroma or
// touching the cache. Only an explicit opt-in can enter Chroma's downloader.
func runtimeLibraryOptions() (string, bool, error) {
	return resolveRuntimeLibrary(os.Getenv("CHROMA_LIB_PATH"), os.Getenv(runtimeDownloadEnv), os.UserHomeDir, runtime.GOOS, runtime.GOARCH)
}

func resolveRuntimeLibrary(explicit, optIn string, homeDir func() (string, error), goos, goarch string) (string, bool, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		if err := checkExplicitRuntime(path, goos); err != nil {
			return "", false, &RuntimeAvailabilityError{Path: path, Explicit: true, Detail: err.Error()}
		}
		return path, false, nil
	}
	fileName := ""
	switch {
	case goos == "linux" && goarch == "amd64":
		fileName = "libchroma_shim.so"
	case goos == "darwin" && goarch == "arm64":
		fileName = "libchroma_shim.dylib"
	case goos == runtimeOSWindows && goarch == "amd64":
		fileName = "chroma_shim.dll"
	default:
		return "", false, &RuntimeAvailabilityError{Detail: fmt.Sprintf("unsupported default runtime platform %s-%s", goos, goarch)}
	}
	home, err := homeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false, &RuntimeAvailabilityError{Detail: fmt.Sprintf("cannot resolve home directory: %v", err)}
	}
	path := filepath.Join(home, ".cache", "chroma", "local_shim", defaultRuntimeVersion, goos+"-"+goarch, fileName)
	if err := checkRuntimeFile(path); err != nil {
		if optIn == "1" {
			// An empty explicit path lets Chroma resolve and download its runtime.
			return "", true, nil
		}
		return "", false, &RuntimeAvailabilityError{Path: path, Detail: err.Error()}
	}
	return path, false, nil
}

// checkExplicitRuntime follows the local loader's directory and extensionless
// candidates. Bare library names can use the system loader search path.
func checkExplicitRuntime(path, goos string) error {
	extension, defaultName := ".so", "libchroma_shim.so"
	switch goos {
	case "darwin":
		extension, defaultName = ".dylib", "libchroma_shim.dylib"
	case runtimeOSWindows:
		extension, defaultName = ".dll", "chroma_shim.dll"
	}
	if !strings.ContainsAny(path, `/\\`) && filepath.Ext(path) == extension {
		return nil
	}
	// #nosec G703 -- CHROMA_LIB_PATH is a trusted local configuration path, not remote input.
	if info, err := os.Stat(path); (err == nil && info.IsDir()) || strings.HasSuffix(path, "/") || strings.HasSuffix(path, `\`) {
		return checkRuntimeFile(filepath.Join(path, defaultName))
	}
	candidates := []string{path}
	if filepath.Ext(path) == "" {
		candidates = append(candidates, path+extension)
		if goos != runtimeOSWindows && !strings.HasPrefix(filepath.Base(path), "lib") {
			candidates = append(candidates, filepath.Join(filepath.Dir(path), "lib"+filepath.Base(path)+extension))
		}
	}
	var firstErr error
	for _, candidate := range candidates {
		if err := checkRuntimeFile(candidate); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func checkRuntimeFile(path string) error {
	info, err := os.Stat(path) // #nosec G703 -- Intentionally inspect the user-configured local runtime path.
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("runtime file is empty or is not a regular file")
	}
	file, err := os.Open(path) // #nosec G703 -- Read-only availability check of the user-configured local runtime.
	if err != nil {
		return err
	}
	return file.Close()
}
