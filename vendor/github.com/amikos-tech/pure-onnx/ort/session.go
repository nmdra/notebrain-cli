package ort

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// AdvancedSession represents an ONNX Runtime inference session
type AdvancedSession struct {
	handle       uintptr
	inputNames   []string
	outputNames  []string
	inputValues  []Value
	outputValues []Value
	runMu        sync.Mutex
}

// NewAdvancedSession creates a new session with specified inputs and outputs.
// Callers retain ownership of input/output values and must keep them alive.
// Values must not be Destroy()'d while this session may still Run().
// If a value is destroyed early, Run() returns a "...value at index N has been destroyed" error.
func NewAdvancedSession(modelPath string, inputNames []string, outputNames []string,
	inputValues []Value, outputValues []Value, options *SessionOptions) (*AdvancedSession, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path cannot be empty")
	}
	if len(inputNames) == 0 {
		return nil, fmt.Errorf("at least one input name is required")
	}
	if len(outputNames) == 0 {
		return nil, fmt.Errorf("at least one output name is required")
	}
	if len(inputNames) != len(inputValues) {
		return nil, fmt.Errorf("input names/values count mismatch: got %d names and %d values", len(inputNames), len(inputValues))
	}
	if len(outputNames) != len(outputValues) {
		return nil, fmt.Errorf("output names/values count mismatch: got %d names and %d values", len(outputNames), len(outputValues))
	}
	if options != nil && options.handle == 0 {
		return nil, fmt.Errorf("session options handle is not initialized")
	}

	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	// Validate values while ortCallMu is held so handles cannot be released concurrently.
	for i, v := range inputValues {
		if err := validateSessionValue(v, "input", i); err != nil {
			return nil, err
		}
	}
	for i, v := range outputValues {
		if err := validateSessionValue(v, "output", i); err != nil {
			return nil, err
		}
	}

	mu.Lock()
	// Safe to snapshot under mu here because ortCallMu.RLock is already held.
	// DestroyEnvironment takes ortCallMu.Lock before it can nil these globals.
	if ortAPI == nil || ortEnv == 0 || createSessionOptionsFunc == nil || releaseSessionOptionsFunc == nil || createSessionFunc == nil {
		mu.Unlock()
		return nil, fmt.Errorf("ONNX Runtime not initialized")
	}
	envHandle := ortEnv
	createSessionOptions := createSessionOptionsFunc
	releaseSessionOptions := releaseSessionOptionsFunc
	createSession := createSessionFunc
	mu.Unlock()

	sessionOptionsHandle := uintptr(0)
	releaseCreatedOptions := false
	if options != nil {
		sessionOptionsHandle = options.handle
	} else {
		status := createSessionOptions(&sessionOptionsHandle)
		if status != 0 {
			errMsg := getErrorMessage(status)
			releaseStatus(status)
			return nil, fmt.Errorf("failed to create session options: %s", errMsg)
		}
		releaseCreatedOptions = true
	}
	if releaseCreatedOptions {
		defer releaseSessionOptions(sessionOptionsHandle)
	}

	modelPathPtr, modelPathBacking, err := goStringToORTChar(modelPath)
	if err != nil {
		return nil, err
	}

	var sessionHandle uintptr
	status := createSession(envHandle, modelPathPtr, sessionOptionsHandle, &sessionHandle)
	// modelPathBacking owns the native char buffer returned by goStringToORTChar.
	// Keep it alive until createSession returns.
	runtime.KeepAlive(modelPathBacking)
	if status != 0 {
		errMsg := getErrorMessage(status)
		releaseStatus(status)
		return nil, fmt.Errorf("failed to create session: %s", errMsg)
	}

	session := &AdvancedSession{
		handle:       sessionHandle,
		inputNames:   cloneStringSlice(inputNames),
		outputNames:  cloneStringSlice(outputNames),
		inputValues:  cloneValueSlice(inputValues),
		outputValues: cloneValueSlice(outputValues),
	}

	runtime.SetFinalizer(session, func(s *AdvancedSession) {
		if err := s.Destroy(); err != nil {
			logFinalizerWarning("WARNING: session finalizer destroy failed: %v", err)
		}
	})

	return session, nil
}

// Run executes inference on the session.
// Calls are intentionally serialized per session instance via runMu because this MVP
// binds fixed input/output value handles onto the session object.
func (s *AdvancedSession) Run() error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}

	// Lock order here is runMu -> ortCallMu -> mu.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	// Holding ortCallMu RLock keeps DestroyEnvironment() from closing the runtime
	// while raw pointers are passed into ORT.
	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	var (
		sessionHandle uintptr
		inputNames    []string
		outputNames   []string
		inputValues   []Value
		outputValues  []Value
		run           func(session uintptr, runOptions uintptr, inputNames *uintptr, inputValues *uintptr, inputLen uintptr, outputNames *uintptr, outputLen uintptr, outputValues *uintptr) uintptr
	)

	// Session-owned fields are guarded by runMu.
	if s.handle == 0 {
		return fmt.Errorf("session has been destroyed")
	}
	if len(s.inputNames) == 0 || len(s.outputNames) == 0 {
		return fmt.Errorf("session is missing input/output names")
	}
	if len(s.inputNames) != len(s.inputValues) {
		return fmt.Errorf("session input names/values count mismatch: got %d names and %d values", len(s.inputNames), len(s.inputValues))
	}
	if len(s.outputNames) != len(s.outputValues) {
		return fmt.Errorf("session output names/values count mismatch: got %d names and %d values", len(s.outputNames), len(s.outputValues))
	}
	sessionHandle = s.handle
	inputNames = s.inputNames
	outputNames = s.outputNames
	inputValues = s.inputValues
	outputValues = s.outputValues

	// Global runtime pointers/functions are guarded by mu.
	// Safe to snapshot under mu here because ortCallMu.RLock is already held.
	// DestroyEnvironment takes ortCallMu.Lock before it can nil these globals.
	mu.Lock()
	if ortAPI == nil || runSessionFunc == nil {
		mu.Unlock()
		return fmt.Errorf("ONNX Runtime not initialized")
	}
	run = runSessionFunc
	mu.Unlock()

	inputNameBackings, inputNamePtrs := makeCStringPointerArray(inputNames)
	outputNameBackings, outputNamePtrs := makeCStringPointerArray(outputNames)

	inputValueHandles, releaseInputValueHandles, err := valuesToHandles(inputValues, "input")
	if err != nil {
		return err
	}
	defer releaseInputValueHandles()

	outputValueHandles, releaseOutputValueHandles, err := valuesToHandles(outputValues, "output")
	if err != nil {
		return err
	}
	defer releaseOutputValueHandles()

	status := run(
		sessionHandle,
		0, // RunOptions not yet implemented
		uintptrSlicePtr(inputNamePtrs),
		uintptrSlicePtr(inputValueHandles),
		uintptr(len(inputValueHandles)),
		uintptrSlicePtr(outputNamePtrs),
		uintptr(len(outputValueHandles)),
		uintptrSlicePtr(outputValueHandles),
	)
	// Keep backing slices alive until ORT returns because runSessionFunc receives raw pointers into them.
	runtime.KeepAlive(inputNameBackings)
	runtime.KeepAlive(outputNameBackings)
	runtime.KeepAlive(inputNamePtrs)
	runtime.KeepAlive(outputNamePtrs)
	runtime.KeepAlive(inputValueHandles)
	runtime.KeepAlive(outputValueHandles)
	if status != 0 {
		errMsg := getErrorMessage(status)
		releaseStatus(status)
		return fmt.Errorf("failed to run inference: %s", errMsg)
	}

	return nil
}

// Destroy releases the session resources
func (s *AdvancedSession) Destroy() error {
	if s == nil {
		return nil
	}

	// Lock order here is runMu -> ortCallMu -> mu.
	// runMu prevents overlap with Run() on this same session.
	// ortCallMu.RLock keeps environment teardown from closing the runtime while
	// this release call is in flight, without stalling unrelated session runs.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	var (
		handle         uintptr
		releaseSession func(uintptr)
	)

	mu.Lock()
	handle = s.handle
	releaseSession = releaseSessionFunc
	s.handle = 0
	s.inputNames = nil
	s.outputNames = nil
	s.inputValues = nil
	s.outputValues = nil
	runtime.SetFinalizer(s, nil)
	mu.Unlock()

	if handle != 0 && releaseSession != nil {
		releaseSession(handle)
	} else if handle != 0 {
		return fmt.Errorf("cannot destroy session: ONNX Runtime release function unavailable (environment may already be destroyed); ensure all tensors and sessions are destroyed before calling DestroyEnvironment()")
	}

	return nil
}

// valueWithORTHandle is intentionally package-private.
// Today, sessions only support Value implementations created by this package.
type valueWithORTHandle interface {
	ortValueHandle() uintptr
}

// valueRunLockable values can provide a stable handle lease for the whole Run() call.
// This lets Destroy() wait only on sessions currently using that specific value.
// Implementations must be comparable so repeated values can be deduplicated safely.
type valueRunLockable interface {
	lockForRun() (uintptr, error)
	unlockForRun()
}

var (
	errValueNil         = errors.New("value is nil")
	errValueDestroyed   = errors.New("value has been destroyed")
	errValueUnsupported = errors.New("unsupported value implementation")
)

func valueHandle(v Value) (uintptr, error) {
	if v == nil {
		return 0, errValueNil
	}
	handleProvider, ok := v.(valueWithORTHandle)
	if !ok {
		return 0, fmt.Errorf("%w %T", errValueUnsupported, v)
	}
	handle := handleProvider.ortValueHandle()
	if handle == 0 {
		return 0, errValueDestroyed
	}
	return handle, nil
}

func validateSessionValue(v Value, role string, index int) error {
	_, err := valueHandle(v)
	if err == nil {
		return nil
	}
	if errors.Is(err, errValueDestroyed) {
		return fmt.Errorf("%s value at index %d has been destroyed", role, index)
	}
	return fmt.Errorf("invalid %s value at index %d: %w", role, index, err)
}

func valuesToHandles(values []Value, role string) ([]uintptr, func(), error) {
	noOpRelease := func() {}
	if len(values) == 0 {
		return nil, noOpRelease, nil
	}
	handles := make([]uintptr, len(values))
	unlockFns := make([]func(), 0, len(values))
	leasedLockables := make(map[any]int, len(values))
	release := func() {
		for i := len(unlockFns) - 1; i >= 0; i-- {
			unlockFns[i]()
		}
	}

	for i, v := range values {
		if lockable, ok := v.(valueRunLockable); ok {
			key, keyOk := comparableIdentityKey(lockable)
			if !keyOk {
				release()
				return nil, noOpRelease, fmt.Errorf("%s value at index %d is invalid: lockable value type %T must be comparable", role, i, v)
			}

			// Lease each comparable lockable only once per Run(). This avoids a deadlock
			// when the same value is bound multiple times and Destroy() queues on the
			// writer side of that value's RWMutex.
			if leasedIndex, exists := leasedLockables[key]; exists {
				handles[i] = handles[leasedIndex]
				continue
			}

			handle, err := lockable.lockForRun()
			if err != nil {
				release()
				if errors.Is(err, errValueDestroyed) {
					return nil, noOpRelease, fmt.Errorf("%s value at index %d has been destroyed", role, i)
				}
				return nil, noOpRelease, fmt.Errorf("%s value at index %d is invalid: %w", role, i, err)
			}
			handles[i] = handle
			unlockFns = append(unlockFns, lockable.unlockForRun)
			leasedLockables[key] = i
			continue
		}

		// Non-lockable values are a compatibility fallback for package-local test doubles.
		// Production Value implementations should provide valueRunLockable leases.
		handle, err := valueHandle(v)
		if err != nil {
			release()
			if errors.Is(err, errValueDestroyed) {
				return nil, noOpRelease, fmt.Errorf("%s value at index %d has been destroyed", role, i)
			}
			return nil, noOpRelease, fmt.Errorf("%s value at index %d is invalid: %w", role, i, err)
		}

		handles[i] = handle
	}
	return handles, release, nil
}

func comparableIdentityKey(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	t := reflect.TypeOf(v)
	if !t.Comparable() {
		return nil, false
	}
	return v, true
}

func cloneStringSlice(input []string) []string {
	if len(input) == 0 {
		// Use nil for optional string collections when there are no entries.
		return nil
	}
	out := make([]string, len(input))
	copy(out, input)
	return out
}

func cloneValueSlice(input []Value) []Value {
	if len(input) == 0 {
		return nil
	}
	out := make([]Value, len(input))
	copy(out, input)
	return out
}

func makeCStringPointerArray(values []string) ([][]byte, []uintptr) {
	if len(values) == 0 {
		return nil, nil
	}

	backings := make([][]byte, len(values))
	ptrs := make([]uintptr, len(values))
	for i, value := range values {
		bytes, ptr := GoToCstring(value)
		backings[i] = bytes
		ptrs[i] = ptr
	}
	return backings, ptrs
}

func uintptrSlicePtr(values []uintptr) *uintptr {
	if len(values) == 0 {
		return nil
	}
	// The returned pointer aliases values' backing array.
	// Callers must KeepAlive(values) until ORT returns.
	// #nosec G103 -- Required for CGO-free FFI to pass pointer arrays to ONNX Runtime C API.
	return (*uintptr)(unsafe.Pointer(unsafe.SliceData(values)))
}
