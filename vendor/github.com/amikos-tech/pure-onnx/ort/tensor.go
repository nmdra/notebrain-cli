package ort

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// Tensor represents a tensor with data of type T
type Tensor[T any] struct {
	shape  Shape
	data   []T
	handle uintptr         // Pointer to OrtValue
	pinner *runtime.Pinner // Pins data backing array while OrtValue may access it.
	runMu  sync.RWMutex    // Coordinates Run() handle leases with Destroy().
}

func (t *Tensor[T]) ortValueHandle() uintptr {
	if t == nil {
		return 0
	}
	t.runMu.RLock()
	handle := t.handle
	t.runMu.RUnlock()
	return handle
}

// NewTensor creates a new tensor with the given shape and data
func NewTensor[T any](shape Shape, data []T) (*Tensor[T], error) {
	elementType, elementSize, err := tensorElementType[T]()
	if err != nil {
		return nil, err
	}

	shapeCopy := cloneShape(shape)
	elementCount, err := shapeElementCount(shapeCopy)
	if err != nil {
		return nil, err
	}
	if len(data) != elementCount {
		return nil, fmt.Errorf("data length mismatch: got %d elements, expected %d for shape %v", len(data), elementCount, shapeCopy)
	}

	return newTensorFromData(shapeCopy, data, elementType, elementSize)
}

// NewEmptyTensor creates a new empty tensor with the given shape
func NewEmptyTensor[T any](shape Shape) (*Tensor[T], error) {
	elementType, elementSize, err := tensorElementType[T]()
	if err != nil {
		return nil, err
	}

	shapeCopy := cloneShape(shape)
	elementCount, err := shapeElementCount(shapeCopy)
	if err != nil {
		return nil, err
	}

	data := make([]T, elementCount)

	return newTensorFromData(shapeCopy, data, elementType, elementSize)
}

func newTensorFromData[T any](shape Shape, data []T, elementType TensorElementDataType, elementSize uintptr) (*Tensor[T], error) {
	dataBytes, err := tensorDataByteSize(len(data), elementSize)
	if err != nil {
		return nil, err
	}

	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	mu.Lock()
	if ortAPI == nil || createMemoryInfoFunc == nil || releaseMemoryInfoFunc == nil || createTensorWithDataAsOrtValueFunc == nil {
		mu.Unlock()
		return nil, fmt.Errorf("ONNX Runtime not initialized")
	}
	createMemoryInfo := createMemoryInfoFunc
	releaseMemoryInfo := releaseMemoryInfoFunc
	createTensorWithData := createTensorWithDataAsOrtValueFunc
	mu.Unlock()

	nameBytes, namePtr := GoToCstring("Cpu")
	var memInfo uintptr
	// TODO: allow caller-configurable allocator/memory type once non-CPU providers are exposed.
	status := createMemoryInfo(namePtr, AllocatorTypeArena, 0, MemTypeCPU, &memInfo)
	runtime.KeepAlive(nameBytes)
	if status != 0 {
		errMsg := getErrorMessage(status)
		releaseStatus(status)
		return nil, fmt.Errorf("failed to create CPU memory info: %s", errMsg)
	}
	defer releaseMemoryInfo(memInfo)

	var dataPtr uintptr
	var pinner *runtime.Pinner
	if len(data) > 0 {
		pinner = &runtime.Pinner{}
		// #nosec G103 -- Required for CGO-free FFI; backing array is pinned for OrtValue lifetime via runtime.Pinner.
		pinner.Pin(unsafe.SliceData(data))
		// #nosec G103 -- Pointer conversion is required to pass the pinned slice buffer to ORT.
		dataPtr = uintptr(unsafe.Pointer(unsafe.SliceData(data)))
	} else {
		// For zero-element tensors, ORT receives a nil data pointer with byte length 0.
		dataPtr = 0
	}

	var valueHandle uintptr
	status = createTensorWithData(memInfo, dataPtr, dataBytes, shapePtr(shape), uintptr(len(shape)), elementType, &valueHandle)
	// ORT reads shape dimensions synchronously during CreateTensorWithDataAsOrtValue call.
	// Keep shape alive for the call; tensor data lifetime is guarded by pinner.
	runtime.KeepAlive(shape)
	if status != 0 {
		if pinner != nil {
			pinner.Unpin()
		}
		errMsg := getErrorMessage(status)
		releaseStatus(status)
		return nil, fmt.Errorf("failed to create tensor: %s", errMsg)
	}

	tensor := &Tensor[T]{
		shape:  shape,
		data:   data,
		handle: valueHandle,
		pinner: pinner,
	}

	// Finalizer is a safety net to avoid leaking OrtValue if callers forget Destroy().
	runtime.SetFinalizer(tensor, func(t *Tensor[T]) {
		if err := t.Destroy(); err != nil {
			logFinalizerWarning("WARNING: tensor finalizer destroy failed: %v", err)
		}
	})

	return tensor, nil
}

// GetData returns the tensor data.
// After Destroy() it returns nil. Calling on a nil receiver also returns nil.
func (t *Tensor[T]) GetData() []T {
	if t == nil {
		return nil
	}
	t.runMu.RLock()
	data := t.data
	t.runMu.RUnlock()
	return data
}

// Shape returns the tensor shape
func (t *Tensor[T]) Shape() Shape {
	if t == nil {
		return nil
	}
	t.runMu.RLock()
	shape := t.shape
	t.runMu.RUnlock()
	return shape
}

// Destroy releases the tensor resources.
//
// Concurrency note: Destroy takes a per-tensor write lock to wait for in-flight
// Run() calls using this specific tensor handle, then releases the ORT value under
// ortCallMu.RLock so environment teardown cannot race the release call.
func (t *Tensor[T]) Destroy() error {
	if t == nil {
		return nil
	}

	var handle uintptr
	var pinner *runtime.Pinner

	// Lock order here is ortCallMu -> mu and ortCallMu -> tensor.runMu.
	// Keep ortCallMu held while releasing the native handle so environment teardown
	// cannot invalidate ORT function pointers mid-call.
	ortCallMu.RLock()
	defer ortCallMu.RUnlock()

	var releaseValue func(uintptr)
	mu.Lock()
	releaseValue = releaseValueFunc
	mu.Unlock()

	t.runMu.Lock()
	handle = t.handle
	pinner = t.pinner
	t.handle = 0
	t.data = nil
	t.shape = nil
	t.pinner = nil
	runtime.SetFinalizer(t, nil)
	t.runMu.Unlock()

	if handle != 0 && releaseValue != nil {
		releaseValue(handle)
	} else if handle != 0 {
		// Safe to unpin: the ORT environment is already destroyed, so nothing
		// will access the backing data through the leaked handle.
		if pinner != nil {
			pinner.Unpin()
		}
		return fmt.Errorf("cannot destroy tensor: ONNX Runtime release function unavailable (environment may already be destroyed); ensure all tensors and sessions are destroyed before calling DestroyEnvironment()")
	}
	if pinner != nil {
		pinner.Unpin()
	}

	return nil
}

func (t *Tensor[T]) lockForRun() (uintptr, error) {
	if t == nil {
		return 0, errValueNil
	}

	t.runMu.RLock()
	handle := t.handle
	if handle == 0 {
		t.runMu.RUnlock()
		return 0, errValueDestroyed
	}

	return handle, nil
}

func (t *Tensor[T]) unlockForRun() {
	if t == nil {
		return
	}
	t.runMu.RUnlock()
}

// Type returns the value type (always ValueTypeTensor for tensors)
func (t *Tensor[T]) Type() ValueType {
	return ValueTypeTensor
}

func cloneShape(shape Shape) Shape {
	if len(shape) == 0 {
		// Keep scalar tensors as non-nil empty shape (rank 0), not nil.
		return Shape{}
	}

	shapeCopy := make(Shape, len(shape))
	copy(shapeCopy, shape)
	return shapeCopy
}

func shapeElementCount(shape Shape) (int, error) {
	maxInt := int(^uint(0) >> 1)

	count := 1
	for i, dim := range shape {
		if dim < 0 {
			return 0, fmt.Errorf("invalid shape dimension at index %d: %d (must be >= 0)", i, dim)
		}

		if dim == 0 {
			// Continue scanning to validate remaining dimensions (for example reject {0, -1}).
			count = 0
			continue
		}

		if count == 0 {
			continue
		}

		if dim > int64(maxInt) {
			return 0, fmt.Errorf("shape dimension at index %d is too large: %d", i, dim)
		}

		dimInt := int(dim)
		if count > maxInt/dimInt {
			return 0, fmt.Errorf("shape %v exceeds maximum supported element count", shape)
		}

		count *= dimInt
	}

	return count, nil
}

// ShapeElementCount returns the total element count for a shape.
// Dimensions must be non-negative; zero dimensions produce a count of zero.
func ShapeElementCount(shape Shape) (int, error) {
	return shapeElementCount(shape)
}

func shapePtr(shape Shape) *int64 {
	if len(shape) == 0 {
		return nil
	}
	// The returned pointer aliases shape's backing array.
	// Callers must KeepAlive(shape) until ORT returns.
	// #nosec G103 -- Safe here because ORT reads shape synchronously in the same call.
	return unsafe.SliceData(shape)
}

func tensorDataByteSize(elementCount int, elementSize uintptr) (uintptr, error) {
	if elementCount < 0 {
		return 0, fmt.Errorf("element count cannot be negative: %d", elementCount)
	}
	if elementCount == 0 {
		return 0, nil
	}
	if elementSize == 0 {
		return 0, fmt.Errorf("element size cannot be zero")
	}

	count := uintptr(elementCount)
	if count > ^uintptr(0)/elementSize {
		return 0, fmt.Errorf("tensor data size overflow: %d elements with element size %d", elementCount, elementSize)
	}

	return count * elementSize, nil
}

// tensorElementType maps Go generic element type T to ONNX tensor element metadata.
// Supported types in this MVP are float32, float64, int32, and int64.
func tensorElementType[T any]() (TensorElementDataType, uintptr, error) {
	var zero T

	switch any(zero).(type) {
	case float32:
		return TensorElementDataTypeFloat, unsafe.Sizeof(zero), nil
	case float64:
		return TensorElementDataTypeDouble, unsafe.Sizeof(zero), nil
	case int32:
		return TensorElementDataTypeInt32, unsafe.Sizeof(zero), nil
	case int64:
		return TensorElementDataTypeInt64, unsafe.Sizeof(zero), nil
	default:
		return TensorElementDataTypeUndefined, 0, fmt.Errorf("unsupported tensor element type %T", zero)
	}
}
