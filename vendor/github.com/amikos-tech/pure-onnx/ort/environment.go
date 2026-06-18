package ort

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	// defaultLogID is the default log identifier used when creating the ONNX Runtime environment
	defaultLogID = "onnx-purego"
)

var (
	// Lock hierarchy across ORT lifecycle and calls:
	// 1) AdvancedSession.runMu (session-local serialization)
	// 2) ortCallMu (RLock for regular ORT calls; Lock for environment init/destroy
	//    and selected object releases that must not overlap in-flight ORT use)
	// 3) mu (global runtime pointers/function snapshots)
	// 4) Tensor.runMu (value-local run lease lock; only acquired while ortCallMu is held)
	//
	// Keep this order to avoid deadlocks.
	mu                                 sync.Mutex
	ortCallMu                          sync.RWMutex
	refCount                           int
	ortLib                             uintptr
	ortAPI                             *OrtApi
	ortEnv                             uintptr
	libPath                            string
	logLevel                           LoggingLevel = LoggingLevelWarning // Default to Warning
	getVersionStringFunc               func() uintptr
	getErrorMessageFunc                func(uintptr) uintptr
	releaseStatusFunc                  func(uintptr)
	createMemoryInfoFunc               func(name uintptr, allocatorType AllocatorType, deviceID int32, memType MemType, out *uintptr) uintptr
	releaseMemoryInfoFunc              func(uintptr)
	createTensorWithDataAsOrtValueFunc func(info uintptr, pData uintptr, pDataLen uintptr, shape *int64, shapeLen uintptr, dataType TensorElementDataType, out *uintptr) uintptr
	releaseValueFunc                   func(uintptr)
	createSessionOptionsFunc           func(out *uintptr) uintptr
	releaseSessionOptionsFunc          func(uintptr)
	createSessionFunc                  func(env uintptr, modelPath uintptr, sessionOptions uintptr, out *uintptr) uintptr
	runSessionFunc                     func(session uintptr, runOptions uintptr, inputNames *uintptr, inputValues *uintptr, inputLen uintptr, outputNames *uintptr, outputLen uintptr, outputValues *uintptr) uintptr
	releaseSessionFunc                 func(uintptr)
)

func clearORTGlobalsLocked() {
	ortAPI = nil
	ortEnv = 0
	getVersionStringFunc = nil
	getErrorMessageFunc = nil
	releaseStatusFunc = nil
	createMemoryInfoFunc = nil
	releaseMemoryInfoFunc = nil
	createTensorWithDataAsOrtValueFunc = nil
	releaseValueFunc = nil
	createSessionOptionsFunc = nil
	releaseSessionOptionsFunc = nil
	createSessionFunc = nil
	runSessionFunc = nil
	releaseSessionFunc = nil
}

// getErrorMessage extracts the error message from an ORT status code.
// Returns empty string if status is 0 (success) or if the function is not initialized.
func getErrorMessage(status uintptr) string {
	if status == 0 || getErrorMessageFunc == nil {
		return ""
	}

	msgPtr := getErrorMessageFunc(status)
	return CstringToGo(msgPtr)
}

// releaseStatus releases an ORT status object to prevent memory leaks.
func releaseStatus(status uintptr) {
	if status == 0 || releaseStatusFunc == nil {
		return
	}

	releaseStatusFunc(status)
}

// InitializeEnvironment initializes the ONNX Runtime environment
func InitializeEnvironment() (err error) {
	ortCallMu.Lock()
	defer ortCallMu.Unlock()

	mu.Lock()
	defer mu.Unlock()

	if refCount > 0 {
		refCount++
		return nil
	}

	if libPath == "" {
		return fmt.Errorf("library path not set, call SetSharedLibraryPath first")
	}

	// Setup centralized cleanup for error paths
	var cleanupNeeded = true
	defer func() {
		if cleanupNeeded {
			if ortLib != 0 {
				if closeErr := closeLibrary(ortLib); closeErr != nil {
					closeErr = fmt.Errorf("failed to close ONNX Runtime library during initialization cleanup: %w", closeErr)
					if err == nil {
						err = closeErr
					} else {
						err = errors.Join(err, closeErr)
					}
				}
				ortLib = 0
			}
			clearORTGlobalsLocked()
		}
	}()

	ortLib, err = loadLibrary(libPath)
	if err != nil {
		return fmt.Errorf("failed to load ONNX Runtime library: %w", err)
	}

	sym, err := getSymbol(ortLib, "OrtGetApiBase")
	if err != nil {
		return fmt.Errorf("failed to get OrtGetApiBase symbol: %w", err)
	}

	var ortGetApiBase func() *OrtApiBase
	purego.RegisterFunc(&ortGetApiBase, sym)
	apiBase := ortGetApiBase()

	purego.RegisterFunc(&getVersionStringFunc, apiBase.GetVersionString)

	var getApi func(uint32) uintptr
	purego.RegisterFunc(&getApi, apiBase.GetApi)
	apiPtr := getApi(ORT_API_VERSION)
	// #nosec G103 -- This unsafe conversion is required for purego FFI.
	// The OrtApi struct layout exactly matches the C API struct returned by GetApi.
	// This pattern is the standard way to use purego for calling C libraries without CGO.
	ortAPI = (*OrtApi)(unsafe.Pointer(apiPtr))

	// Register frequently-used API functions once to avoid repeated RegisterFunc calls
	purego.RegisterFunc(&getErrorMessageFunc, ortAPI.GetErrorMessage)
	purego.RegisterFunc(&releaseStatusFunc, ortAPI.ReleaseStatus)
	purego.RegisterFunc(&createMemoryInfoFunc, ortAPI.CreateMemoryInfo)
	purego.RegisterFunc(&releaseMemoryInfoFunc, ortAPI.ReleaseMemoryInfo)
	purego.RegisterFunc(&createTensorWithDataAsOrtValueFunc, ortAPI.CreateTensorWithDataAsOrtValue)
	purego.RegisterFunc(&releaseValueFunc, ortAPI.ReleaseValue)
	purego.RegisterFunc(&createSessionOptionsFunc, ortAPI.CreateSessionOptions)
	purego.RegisterFunc(&releaseSessionOptionsFunc, ortAPI.ReleaseSessionOptions)
	purego.RegisterFunc(&createSessionFunc, ortAPI.CreateSession)
	purego.RegisterFunc(&runSessionFunc, ortAPI.Run)
	purego.RegisterFunc(&releaseSessionFunc, ortAPI.ReleaseSession)

	// Validate ONNX Runtime version (warn if mismatch, unless explicitly skipped)
	if os.Getenv("ONNXRUNTIME_SKIP_VERSION_CHECK") == "" {
		versionPtr := getVersionStringFunc()
		version := CstringToGo(versionPtr)

		// Parse version string (format: "1.XX.Y")
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			minor, err := strconv.Atoi(parts[1])
			if err == nil && minor < 22 {
				log.Printf("WARNING: ONNX Runtime version %s is older than 1.22.0 (API version %d). "+
					"This package was built against 1.22.0+. You may encounter compatibility issues. "+
					"To suppress this warning, set ONNXRUNTIME_SKIP_VERSION_CHECK=1", version, ORT_API_VERSION)
			}
		}
	}

	var createEnv func(logLevel int32, logID uintptr, out *uintptr) uintptr
	purego.RegisterFunc(&createEnv, ortAPI.CreateEnv)

	logIDBytes, logIDPtr := GoToCstring(defaultLogID)
	// #nosec G115 -- LoggingLevel values are constrained to 0-4 by type definition, no overflow possible
	status := createEnv(int32(logLevel), logIDPtr, &ortEnv)
	runtime.KeepAlive(logIDBytes) // Prevent GC from collecting bytes during C call
	if status != 0 {
		errMsg := getErrorMessage(status)
		releaseStatus(status)
		return fmt.Errorf("failed to create ONNX Runtime environment: %s", errMsg)
	}

	// Success - prevent cleanup
	cleanupNeeded = false
	refCount = 1
	return nil
}

// DestroyEnvironment cleans up the ONNX Runtime environment
func DestroyEnvironment() error {
	ortCallMu.Lock()
	defer ortCallMu.Unlock()

	mu.Lock()
	defer mu.Unlock()

	if refCount == 0 {
		return nil
	}

	refCount--
	if refCount > 0 {
		return nil
	}

	if ortAPI != nil && ortEnv != 0 {
		// Now that we have the complete OrtApi struct layout (all 305 functions),
		// we can properly call ReleaseEnv
		var releaseEnv func(uintptr)
		purego.RegisterFunc(&releaseEnv, ortAPI.ReleaseEnv)
		releaseEnv(ortEnv)
		ortEnv = 0
	}

	var closeErr error
	if ortLib != 0 {
		if err := closeLibrary(ortLib); err != nil {
			closeErr = fmt.Errorf("failed to close ONNX Runtime library: %w", err)
		}
		// Clear the handle even when close fails to avoid reusing stale symbols.
		ortLib = 0
	}

	// Always clear function pointers/state after environment destruction. If
	// closeLibrary fails, stale pointers must still be removed.
	clearORTGlobalsLocked()
	return closeErr
}

// IsInitialized returns true if the environment is initialized
func IsInitialized() bool {
	mu.Lock()
	defer mu.Unlock()
	return refCount > 0
}

// SetSharedLibraryPath sets the path to the ONNX Runtime shared library.
// This must be called before InitializeEnvironment().
// Returns an error if the environment is already initialized.
func SetSharedLibraryPath(path string) error {
	mu.Lock()
	defer mu.Unlock()
	if refCount > 0 {
		return fmt.Errorf("cannot change library path after environment is initialized")
	}
	libPath = path
	return nil
}

// SetLogLevel sets the logging level for the ONNX Runtime environment.
// This must be called before InitializeEnvironment() to take effect.
// Valid levels are: LoggingLevelVerbose, LoggingLevelInfo, LoggingLevelWarning, LoggingLevelError, LoggingLevelFatal.
// Default is LoggingLevelWarning.
// Returns an error if the environment is already initialized.
func SetLogLevel(level LoggingLevel) error {
	mu.Lock()
	defer mu.Unlock()
	if refCount > 0 {
		return fmt.Errorf("cannot change log level after environment is initialized")
	}
	logLevel = level
	return nil
}

// GetVersionString returns the ONNX Runtime version string.
// Returns "0.0.0-dev" if the environment is not initialized.
//
// Thread-safety: This function is safe to call concurrently from multiple goroutines.
// It acquires ortCallMu.RLock to prevent concurrent environment teardown, snapshots
// the function pointer under mu, then calls it after releasing mu.
func GetVersionString() string {
	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	mu.Lock()
	if refCount == 0 || getVersionStringFunc == nil {
		mu.Unlock()
		return "0.0.0-dev"
	}
	versionStringFunc := getVersionStringFunc
	mu.Unlock()

	versionPtr := versionStringFunc()
	return CstringToGo(versionPtr)
}
