package chroma

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
)

// CompactCollectionRequest identifies one collection to compact, optionally scoped by tenant and database.
type CompactCollectionRequest struct {
	Name         string `json:"name"`
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// CompactAllRequest scopes global compaction, optionally by tenant and database.
type CompactAllRequest struct {
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// CompactionCollectionResult captures per-collection compaction stats.
type CompactionCollectionResult struct {
	CollectionID          string  `json:"collection_id"`
	Name                  string  `json:"name"`
	TenantID              string  `json:"tenant_id"`
	DatabaseName          string  `json:"database_name"`
	PendingOpsBefore      *uint64 `json:"pending_ops_before,omitempty"`
	PendingOpsAfter       *uint64 `json:"pending_ops_after,omitempty"`
	PendingOpsBeforeError string  `json:"pending_ops_before_error,omitempty"`
	PendingOpsAfterError  string  `json:"pending_ops_after_error,omitempty"`
	Error                 string  `json:"error,omitempty"`
}

// CompactionResult captures explicit compaction execution metadata.
type CompactionResult struct {
	// CollectionCount is the number of collections attempted, including entries with per-collection errors.
	CollectionCount       uint32                       `json:"collection_count"`
	DurationMS            uint64                       `json:"duration_ms"`
	PendingOpsBeforeTotal uint64                       `json:"pending_ops_before_total"`
	PendingOpsAfterTotal  uint64                       `json:"pending_ops_after_total"`
	Collections           []CompactionCollectionResult `json:"collections"`
}

func (r CompactCollectionRequest) validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if databaseName := strings.TrimSpace(r.DatabaseName); databaseName != "" && len(databaseName) < 3 {
		return errors.New("database_name must be at least 3 characters")
	}
	return nil
}

func (r CompactAllRequest) validate() error {
	if databaseName := strings.TrimSpace(r.DatabaseName); databaseName != "" && len(databaseName) < 3 {
		return errors.New("database_name must be at least 3 characters")
	}
	return nil
}

// CompactCollection runs explicit compaction for one collection in embedded mode.
func (e *Embedded) CompactCollection(request CompactCollectionRequest) (*CompactionResult, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}
	if err := request.validate(); err != nil {
		return nil, err
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedCompactCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response CompactionResult
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode compact collection response")
	}
	return &response, nil
}

// CompactAll runs explicit compaction for all collections in embedded mode.
// Per-collection failures are reported in CompactionResult.Collections[i].Error.
func (e *Embedded) CompactAll(request CompactAllRequest) (*CompactionResult, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}
	if err := request.validate(); err != nil {
		return nil, err
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedCompactAll(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response CompactionResult
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode compact all response")
	}
	return &response, nil
}

// CompactCollection runs explicit compaction for one collection in managed server mode.
// The server is restarted after the operation, even if compaction fails.
// The server is unavailable while compaction is running.
// Backups and server compaction are mutually exclusive and serialized.
// If compaction succeeds but cleanup or restart fails, this returns a non-nil CompactionResult and a non-nil error.
func (s *Server) CompactCollection(request CompactCollectionRequest) (*CompactionResult, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}
	if err := request.validate(); err != nil {
		return nil, err
	}
	return s.runCompaction(func(embedded *Embedded) (*CompactionResult, error) {
		return embedded.CompactCollection(request)
	})
}

// CompactAll runs explicit compaction for all collections in managed server mode.
// The server is restarted after the operation, even if compaction fails.
// The server is unavailable while compaction is running.
// Backups and server compaction are mutually exclusive and serialized.
// Per-collection failures are reported in CompactionResult.Collections[i].Error.
// If compaction succeeds but cleanup or restart fails, this returns a non-nil CompactionResult and a non-nil error.
func (s *Server) CompactAll(request CompactAllRequest) (*CompactionResult, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}
	if err := request.validate(); err != nil {
		return nil, err
	}
	return s.runCompaction(func(embedded *Embedded) (*CompactionResult, error) {
		return embedded.CompactAll(request)
	})
}

func (s *Server) runCompaction(run func(*Embedded) (*CompactionResult, error)) (*CompactionResult, error) {
	// Serialize maintenance operations and keep lock ordering stable: backupMu -> stateMu -> ffiMu.
	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	config, _, err := s.snapshotBackupInputs()
	if err != nil {
		return nil, err
	}
	if err := s.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to stop server before compaction")
	}

	//nolint:staticcheck // S1016: keep explicit field mapping so server and embedded config types can evolve independently.
	embedded, startErr := StartEmbedded(StartEmbeddedConfig{
		ConfigPath:   config.ConfigPath,
		ConfigString: config.ConfigString,
	})
	if startErr != nil {
		restartErr := s.restartFromConfig(config)
		if restartErr != nil {
			return nil, fmt.Errorf("failed to start temporary embedded runtime for compaction: %w; restart failed: %w; server remains stopped", startErr, restartErr)
		}
		return nil, errors.Wrap(startErr, "failed to start temporary embedded runtime for compaction")
	}

	result, runErr := run(embedded)
	closeErr := embedded.Close()

	restartErr := s.restartFromConfig(config)
	switch {
	case runErr != nil:
		if closeErr != nil {
			runErr = fmt.Errorf("%w; temporary embedded runtime close failed: %w", runErr, closeErr)
		}
		if restartErr != nil {
			return nil, fmt.Errorf("%w; restart failed: %w; server remains stopped", runErr, restartErr)
		}
		return nil, runErr
	case closeErr != nil:
		if restartErr != nil {
			return result, fmt.Errorf("compaction completed but temporary embedded runtime close failed: %w; restart failed: %w; server remains stopped", closeErr, restartErr)
		}
		return result, errors.Wrap(closeErr, "compaction completed but failed to close temporary embedded runtime")
	case restartErr != nil:
		return result, errors.Wrap(restartErr, "compaction completed but server restart failed; server remains stopped")
	default:
		return result, nil
	}
}
