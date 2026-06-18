package chroma

import (
	"errors"
	"fmt"
)

// Error codes matching the Rust shim
const (
	Success           int32 = 0
	ErrNullInput      int32 = -1
	ErrInvalidUTF8    int32 = -2
	ErrConfigParse    int32 = -3
	ErrServerStart    int32 = -4
	ErrInvalidHandle  int32 = -5
	ErrRuntimeCreate  int32 = -6
	ErrAlreadyStopped int32 = -7
	ErrOperation      int32 = -8
)

var (
	ErrNullPointer        = errors.New("null pointer returned from library")
	ErrLibraryNotLoaded   = errors.New("library not loaded")
	ErrServerNotStarted   = errors.New("server not started")
	ErrServerAlreadyStop  = errors.New("server already stopped")
	ErrEmbeddedNotStarted = errors.New("embedded mode not started")
)

func errorFromCode(code int32, errMsg string) error {
	switch code {
	case Success:
		return nil
	case ErrNullInput:
		return fmt.Errorf("null input: %s", errMsg)
	case ErrInvalidUTF8:
		return fmt.Errorf("invalid UTF-8: %s", errMsg)
	case ErrConfigParse:
		return fmt.Errorf("config parse error: %s", errMsg)
	case ErrServerStart:
		return fmt.Errorf("server start error: %s", errMsg)
	case ErrInvalidHandle:
		return fmt.Errorf("invalid handle: %s", errMsg)
	case ErrRuntimeCreate:
		return fmt.Errorf("runtime create error: %s", errMsg)
	case ErrAlreadyStopped:
		return ErrServerAlreadyStop
	case ErrOperation:
		return fmt.Errorf("operation failed: %s", errMsg)
	default:
		return fmt.Errorf("unknown error (code %d): %s", code, errMsg)
	}
}
