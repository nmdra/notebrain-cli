package embedder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	modelDirectoryName = "all-MiniLM-L6-v2"
	modelFileName      = "model.onnx"
	tokenizerFileName  = "tokenizer.json" //nolint:gosec // fixed public model artifact name, not a credential
	lockFileName       = ".download.lock"
)

// ErrModelUnavailable identifies an unavailable local ONNX embedding model.
// Callers can use errors.Is to distinguish this condition from other
// embedder initialization failures.
var ErrModelUnavailable = errors.New("ONNX embedding model is unavailable")

// AvailabilityReason identifies why the local ONNX model cannot be used.
type AvailabilityReason string

const (
	AvailabilityConfiguration    AvailabilityReason = "configuration"
	AvailabilityMissingArtifacts AvailabilityReason = "missing_artifacts"
	AvailabilityDownloadActive   AvailabilityReason = "download_active"
	AvailabilityDownloadStale    AvailabilityReason = "download_stale"
	AvailabilityInvalidLock      AvailabilityReason = "invalid_lock"
)

// ModelAvailabilityError reports a model cache state that must be repaired or
// allowed to finish before the local embedder can be initialized.
type ModelAvailabilityError struct {
	Reason        AvailabilityReason
	ModelPath     string
	TokenizerPath string
	LockPath      string
	Missing       []string
	Lock          *DownloadLock
	Detail        string
}

// DownloadLock describes the ONNX model download lock, when one exists.
type DownloadLock struct {
	Path    string
	PID     int
	Active  bool
	Invalid bool
	Age     time.Duration
	Detail  string
}

func (e *ModelAvailabilityError) Error() string {
	if e == nil {
		return ErrModelUnavailable.Error()
	}

	var b strings.Builder
	b.WriteString(ErrModelUnavailable.Error())
	if len(e.Missing) > 0 {
		b.WriteString("; missing or empty files: ")
		b.WriteString(strings.Join(e.Missing, ", "))
	}

	switch e.Reason {
	case AvailabilityDownloadActive:
		pid := 0
		if e.Lock != nil {
			pid = e.Lock.PID
		}
		b.WriteString("; download lock ")
		b.WriteString(strconv.Quote(e.LockPath))
		b.WriteString(" is held by active PID ")
		b.WriteString(strconv.Itoa(pid))
		b.WriteString("; wait for that download to finish")
	case AvailabilityDownloadStale:
		pid := 0
		if e.Lock != nil {
			pid = e.Lock.PID
		}
		b.WriteString("; download lock ")
		b.WriteString(strconv.Quote(e.LockPath))
		b.WriteString(" refers to PID ")
		b.WriteString(strconv.Itoa(pid))
		b.WriteString(", which is not running; remove the lock only after confirming no download is active")
	case AvailabilityInvalidLock:
		b.WriteString("; download lock ")
		b.WriteString(strconv.Quote(e.LockPath))
		b.WriteString(" has unreadable metadata: ")
		b.WriteString(e.Detail)
	case AvailabilityConfiguration:
		if e.Detail != "" {
			b.WriteString("; ")
			b.WriteString(e.Detail)
		}
	default:
		b.WriteString("; run `notebrain doctor` for diagnostics and model recovery instructions")
	}
	return b.String()
}

func (e *ModelAvailabilityError) Unwrap() error { return ErrModelUnavailable }

type modelPaths struct {
	cacheDir      string
	modelPath     string
	tokenizerPath string
	lockPath      string
}

func defaultModelPaths() (modelPaths, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return modelPaths{}, fmt.Errorf("resolve home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "chroma", "onnx_models")
	modelDir := filepath.Join(cacheDir, modelDirectoryName, "onnx")
	return modelPaths{
		cacheDir:      cacheDir,
		modelPath:     filepath.Join(modelDir, modelFileName),
		tokenizerPath: filepath.Join(modelDir, tokenizerFileName),
		lockPath:      filepath.Join(cacheDir, lockFileName),
	}, nil
}

// CheckModelAvailability verifies that the local MiniLM artifacts are ready
// before Chroma's ONNX constructor can attempt a download.
func CheckModelAvailability() error {
	paths, err := defaultModelPaths()
	if err != nil {
		return &ModelAvailabilityError{
			Reason: AvailabilityConfiguration,
			Detail: err.Error(),
		}
	}
	if err := checkModelAvailability(paths, processIsRunning); err != nil {
		return fmt.Errorf("check ONNX model availability: %w", err)
	}
	return nil
}

func checkModelAvailability(paths modelPaths, processRunning func(int) bool) error {
	lock := inspectDownloadLock(paths.lockPath, processRunning)
	if lock != nil {
		availabilityErr := &ModelAvailabilityError{
			ModelPath:     paths.modelPath,
			TokenizerPath: paths.tokenizerPath,
			LockPath:      paths.lockPath,
			Lock:          lock,
		}
		switch {
		case lock.Invalid:
			availabilityErr.Reason = AvailabilityInvalidLock
			availabilityErr.Detail = lock.Detail
		case lock.Active:
			availabilityErr.Reason = AvailabilityDownloadActive
		default:
			availabilityErr.Reason = AvailabilityDownloadStale
		}
		availabilityErr.Missing = missingArtifacts(paths)
		return availabilityErr
	}

	missing := missingArtifacts(paths)
	if len(missing) == 0 {
		return nil
	}
	return &ModelAvailabilityError{
		Reason:        AvailabilityMissingArtifacts,
		ModelPath:     paths.modelPath,
		TokenizerPath: paths.tokenizerPath,
		LockPath:      paths.lockPath,
		Missing:       missing,
	}
}

func missingArtifacts(paths modelPaths) []string {
	missing := make([]string, 0, 2)
	for _, path := range []string{paths.modelPath, paths.tokenizerPath} {
		if !isReadableNonEmptyFile(path) {
			missing = append(missing, path)
		}
	}
	return missing
}

func isReadableNonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	return f.Close() == nil
}

func inspectDownloadLock(path string, processRunning func(int) bool) *DownloadLock {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	lock := &DownloadLock{Path: path}
	if err != nil {
		lock.Invalid = true
		lock.Detail = err.Error()
		return lock
	}
	if age := time.Since(info.ModTime()); age > 0 {
		lock.Age = age
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		lock.Invalid = true
		lock.Detail = err.Error()
		return lock
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil || pid <= 0 {
		lock.Invalid = true
		lock.Detail = fmt.Sprintf("expected a positive process ID, got %q", strings.TrimSpace(string(contents)))
		return lock
	}

	lock.PID = pid
	if processRunning == nil {
		processRunning = processIsRunning
	}
	lock.Active = processRunning(pid)
	return lock
}

func processIsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
