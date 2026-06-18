package ort

const (
	// ORT_API_VERSION is the ONNX Runtime API version this library is built against.
	// Currently set to 22, which corresponds to ONNX Runtime 1.22.0+ (latest stable).
	//
	// Strategy: We use the latest stable ONNX Runtime version as our baseline.
	// When new ONNX Runtime versions are released, we will update this in a separate PR
	// and add support for new API functions incrementally.
	//
	// This must match the major version of your installed ONNX Runtime library.
	ORT_API_VERSION = 22
)

// LoggingLevel represents the logging verbosity level
type LoggingLevel int

const (
	LoggingLevelVerbose LoggingLevel = iota
	LoggingLevelInfo
	LoggingLevelWarning
	LoggingLevelError
	LoggingLevelFatal
)

// ErrorCode represents ONNX Runtime error codes
type ErrorCode int

const (
	ErrorCodeOK ErrorCode = iota
	ErrorCodeFail
	ErrorCodeInvalidArgument
	ErrorCodeNoSuchFile
	ErrorCodeNoModel
	ErrorCodeEngineError
	ErrorCodeRuntimeException
	ErrorCodeInvalidProtobuf
	ErrorCodeModelLoaded
	ErrorCodeNotImplemented
	ErrorCodeInvalidGraph
	ErrorCodeEPFail
	ErrorCodeModelLoadCanceled
	ErrorCodeModelRequiresCompilation
)

// TensorElementDataType represents the data type of tensor elements
type TensorElementDataType int

const (
	TensorElementDataTypeUndefined TensorElementDataType = iota
	TensorElementDataTypeFloat
	TensorElementDataTypeUint8
	TensorElementDataTypeInt8
	TensorElementDataTypeUint16
	TensorElementDataTypeInt16
	TensorElementDataTypeInt32
	TensorElementDataTypeInt64
	TensorElementDataTypeString
	TensorElementDataTypeBool
	TensorElementDataTypeFloat16
	TensorElementDataTypeDouble
	TensorElementDataTypeUint32
	TensorElementDataTypeUint64
	TensorElementDataTypeComplex64
	TensorElementDataTypeComplex128
	TensorElementDataTypeBFloat16
	TensorElementDataTypeFloat8E4M3FN
	TensorElementDataTypeFloat8E4M3FNUZ
	TensorElementDataTypeFloat8E5M2
	TensorElementDataTypeFloat8E5M2FNUZ
	TensorElementDataTypeUint4
	TensorElementDataTypeInt4
)

// AllocatorType represents the type of memory allocator
type AllocatorType int

const (
	AllocatorTypeInvalid AllocatorType = -1
	AllocatorTypeDevice  AllocatorType = 0
	AllocatorTypeArena   AllocatorType = 1
)

// MemType represents memory types for allocated memory
type MemType int

const (
	MemTypeCPUInput  MemType = -2
	MemTypeCPUOutput MemType = -1
	MemTypeCPU       MemType = MemTypeCPUOutput
	MemTypeDefault   MemType = 0
)

// GraphOptimizationLevel represents the level of graph optimizations
type GraphOptimizationLevel int

const (
	GraphOptimizationLevelDisableAll GraphOptimizationLevel = iota
	GraphOptimizationLevelEnableBasic
	GraphOptimizationLevelEnableExtended
	GraphOptimizationLevelEnableAll
)

// ExecutionMode represents the execution mode for the session
type ExecutionMode int

const (
	ExecutionModeSequential ExecutionMode = iota
	ExecutionModeParallel
)

// ONNXType represents the type of an ONNX value
type ONNXType int

const (
	ONNXTypeUnknown ONNXType = iota
	ONNXTypeTensor
	ONNXTypeSequence
	ONNXTypeMap
	ONNXTypeOpaque
	ONNXTypeSparseMap
	ONNXTypeOptional
)
