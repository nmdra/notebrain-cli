package ort

// OrtApiBase represents the base API structure
type OrtApiBase struct {
	GetApi           uintptr
	GetVersionString uintptr
}

// OrtApi is defined in ortapi_generated.go (auto-generated from C header)

// Status represents an ONNX Runtime status
// Thread-safe: Status can be shared across goroutines for read operations
type Status struct {
	handle uintptr // Pointer to OrtStatus
}

// IsOK returns true if the status represents success
func (s *Status) IsOK() bool {
	return s.handle == 0
}

// GetErrorCode returns the error code from the status
// TODO: This method is not fully implemented yet - currently returns ErrorCodeFail for any error
func (s *Status) GetErrorCode() ErrorCode {
	if s.IsOK() {
		return ErrorCodeOK
	}
	// TODO: Implement actual error code retrieval using OrtApi.GetErrorCode
	return ErrorCodeFail
}

// GetErrorMessage returns the error message from the status
// TODO: This method is not fully implemented yet - currently returns generic message
func (s *Status) GetErrorMessage() string {
	if s.IsOK() {
		return ""
	}
	// TODO: Implement actual error message retrieval using OrtApi.GetErrorMessage
	return "Error occurred"
}

// Environment represents an ONNX Runtime environment
// Thread-safe: Environment is thread-safe and can be shared across multiple sessions
type Environment struct {
	handle       uintptr // Pointer to OrtEnv
	loggingLevel LoggingLevel
	logID        string
}

// Session represents an ONNX Runtime session for model inference
// Thread-safe: Session.Run() is thread-safe, multiple threads can call Run() simultaneously
type Session struct {
	handle      uintptr // Pointer to OrtSession
	inputNames  []string
	outputNames []string
	inputCount  int
	outputCount int
}

// Value represents an ONNX Runtime value (tensor, sequence, map, etc.).
// Sessions currently only accept Value implementations created by this package.
type Value interface {
	// Destroy releases the underlying resources
	Destroy() error
	// Type returns the type of the value
	Type() ValueType
}

// ValueType represents the type of an ONNX Runtime value
type ValueType int

const (
	ValueTypeUnknown ValueType = iota
	ValueTypeTensor
	ValueTypeSequence
	ValueTypeMap
	ValueTypeOpaque
	ValueTypeOptional
)

// Shape represents the shape of a tensor
type Shape []int64

// NewShape creates a new shape from dimensions
func NewShape(dims ...int64) Shape {
	return Shape(dims)
}

// SessionOptions represents options for creating a session.
// It is not safe to mutate a SessionOptions instance concurrently with session creation.
type SessionOptions struct {
	handle                 uintptr // Pointer to OrtSessionOptions
	graphOptimizationLevel GraphOptimizationLevel
	executionMode          ExecutionMode
	interOpNumThreads      int
	intraOpNumThreads      int
	logSeverityLevel       LoggingLevel
	logVerbosityLevel      int
	logID                  string
	enableCPUMemArena      bool
	enableMemPattern       bool
	enableProfiling        bool
	optimizedModelFilePath string
}

// MemoryInfo represents memory allocation information
type MemoryInfo struct {
	handle        uintptr // Pointer to OrtMemoryInfo
	name          string
	memType       MemType
	allocatorType AllocatorType
	deviceID      int
}

// TypeInfo represents type information for an ONNX value
type TypeInfo struct {
	handle uintptr // Pointer to OrtTypeInfo
}

// TensorTypeAndShapeInfo represents tensor type and shape information
type TensorTypeAndShapeInfo struct {
	handle      uintptr // Pointer to OrtTensorTypeAndShapeInfo
	elementType TensorElementDataType
	shape       Shape
}

// RunOptions represents options for running inference
type RunOptions struct {
	handle            uintptr // Pointer to OrtRunOptions
	logVerbosityLevel int
	logSeverityLevel  LoggingLevel
	runTag            string
	terminate         bool
}

// CustomOpDomain represents a custom operator domain
type CustomOpDomain struct {
	handle uintptr // Pointer to OrtCustomOpDomain
	domain string
}
