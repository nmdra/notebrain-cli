package chroma

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

// WALPruneCollectionResult captures per-collection prune stats and errors.
type WALPruneCollectionResult struct {
	CollectionID    string  `json:"collection_id"`
	Name            string  `json:"name"`
	TenantID        string  `json:"tenant_id"`
	DatabaseName    string  `json:"database_name"`
	SafeSeqCutoff   *uint64 `json:"safe_seq_cutoff,omitempty"`
	CandidateSeqMin *uint64 `json:"candidate_seq_min,omitempty"`
	CandidateSeqMax *uint64 `json:"candidate_seq_max,omitempty"`
	PrunedSeqMin    *uint64 `json:"pruned_seq_min,omitempty"`
	PrunedSeqMax    *uint64 `json:"pruned_seq_max,omitempty"`
	CandidateCount  uint64  `json:"candidate_count"`
	CandidateBytes  uint64  `json:"candidate_bytes"`
	PrunedCount     uint64  `json:"pruned_count"`
	PrunedBytes     uint64  `json:"pruned_bytes"`
	Error           string  `json:"error,omitempty"`
}

// WALPruneResult captures WAL prune execution metadata and totals.
type WALPruneResult struct {
	CollectionCount     uint32                     `json:"collection_count"`
	DurationMS          uint64                     `json:"duration_ms"`
	DryRun              bool                       `json:"dry_run"`
	VacuumRequested     bool                       `json:"vacuum_requested"`
	VacuumExecuted      bool                       `json:"vacuum_executed"`
	Warning             string                     `json:"warning,omitempty"`
	CandidateCountTotal uint64                     `json:"candidate_count_total"`
	CandidateBytesTotal uint64                     `json:"candidate_bytes_total"`
	PrunedCountTotal    uint64                     `json:"pruned_count_total"`
	PrunedBytesTotal    uint64                     `json:"pruned_bytes_total"`
	Collections         []WALPruneCollectionResult `json:"collections"`
}

type walPruneCollectionRequest struct {
	Name               string  `json:"name"`
	TenantID           string  `json:"tenant_id,omitempty"`
	DatabaseName       string  `json:"database_name,omitempty"`
	DryRun             bool    `json:"dry_run,omitempty"`
	Vacuum             bool    `json:"vacuum,omitempty"`
	MaxAgeSeconds      *uint64 `json:"max_age_seconds,omitempty"`
	MaxBytes           *uint64 `json:"max_bytes,omitempty"`
	WatermarkHighBytes *uint64 `json:"watermark_high_bytes,omitempty"`
	WatermarkLowBytes  *uint64 `json:"watermark_low_bytes,omitempty"`
}

type walPruneAllRequest struct {
	TenantID           string  `json:"tenant_id,omitempty"`
	DatabaseName       string  `json:"database_name,omitempty"`
	DryRun             bool    `json:"dry_run,omitempty"`
	Vacuum             bool    `json:"vacuum,omitempty"`
	MaxAgeSeconds      *uint64 `json:"max_age_seconds,omitempty"`
	MaxBytes           *uint64 `json:"max_bytes,omitempty"`
	WatermarkHighBytes *uint64 `json:"watermark_high_bytes,omitempty"`
	WatermarkLowBytes  *uint64 `json:"watermark_low_bytes,omitempty"`
}

// WALPruneOption applies optional WAL prune scope, mode, and policy settings.
type WALPruneOption interface {
	apply(*walPruneCallOptions) error
}

type walPruneOptionFunc func(*walPruneCallOptions) error

func (f walPruneOptionFunc) apply(options *walPruneCallOptions) error {
	return f(options)
}

type walPruneCallOptions struct {
	tenantID           string
	databaseName       string
	dryRun             bool
	vacuum             bool
	maxAgeSeconds      *uint64
	maxBytes           *uint64
	watermarkHighBytes *uint64
	watermarkLowBytes  *uint64
}

func (o walPruneCallOptions) hasPolicy() bool {
	return o.maxAgeSeconds != nil || o.maxBytes != nil || (o.watermarkHighBytes != nil && o.watermarkLowBytes != nil)
}

func (o walPruneCallOptions) validate() error {
	if o.databaseName != "" && len(o.databaseName) < 3 {
		return errors.New("database_name must be at least 3 characters")
	}
	if o.tenantID != "" && len(o.tenantID) < 3 {
		return errors.New("tenant_id must be at least 3 characters")
	}

	if o.maxAgeSeconds != nil && *o.maxAgeSeconds == 0 {
		return errors.New("max_age_seconds must be greater than 0")
	}
	if (o.watermarkHighBytes == nil) != (o.watermarkLowBytes == nil) {
		return errors.New("wal prune watermark requires both high and low bytes")
	}
	if o.watermarkHighBytes != nil && o.watermarkLowBytes != nil && *o.watermarkLowBytes > *o.watermarkHighBytes {
		return errors.New("wal prune watermark low bytes must be less than or equal to high bytes")
	}
	if !o.dryRun && !o.hasPolicy() {
		return errors.New("at least one WAL prune policy is required unless dry-run is enabled")
	}
	return nil
}

// WithWALPruneTenantID scopes WAL prune collection lookup and all-collection traversal to a tenant.
func WithWALPruneTenantID(tenantID string) WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		options.tenantID = strings.TrimSpace(tenantID)
		return nil
	})
}

// WithWALPruneDatabaseName scopes WAL prune collection lookup and all-collection traversal to a database.
func WithWALPruneDatabaseName(databaseName string) WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		options.databaseName = strings.TrimSpace(databaseName)
		return nil
	})
}

// WithWALPruneDryRun enables projection mode and does not mutate WAL rows.
func WithWALPruneDryRun() WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		options.dryRun = true
		return nil
	})
}

// WithWALPruneVacuum requests SQLite VACUUM after prune execution.
// VACUUM is ignored in dry-run mode.
func WithWALPruneVacuum() WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		options.vacuum = true
		return nil
	})
}

// WithWALPruneMaxAge prunes rows older than maxAge.
func WithWALPruneMaxAge(maxAge time.Duration) WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		if maxAge <= 0 {
			return errors.New("max_age must be greater than 0")
		}

		seconds := maxAge / time.Second
		if maxAge%time.Second != 0 {
			seconds++
		}
		if seconds <= 0 {
			seconds = 1
		}
		value := uint64(seconds)
		options.maxAgeSeconds = &value
		return nil
	})
}

// WithWALPruneMaxBytes prunes oldest rows until candidate bytes are within maxBytes.
// A value of 0 means "prune all safety-eligible candidate rows" for this policy.
func WithWALPruneMaxBytes(maxBytes uint64) WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		value := maxBytes
		options.maxBytes = &value
		return nil
	})
}

// WithWALPruneWatermark configures high/low bytes watermark pruning.
// When candidate bytes exceed highBytes, oldest rows are pruned until candidate bytes are at or below lowBytes.
func WithWALPruneWatermark(highBytes, lowBytes uint64) WALPruneOption {
	return walPruneOptionFunc(func(options *walPruneCallOptions) error {
		if lowBytes > highBytes {
			return errors.New("wal prune watermark low bytes must be less than or equal to high bytes")
		}
		high := highBytes
		low := lowBytes
		options.watermarkHighBytes = &high
		options.watermarkLowBytes = &low
		return nil
	})
}

func resolveWALPruneCallOptions(options []WALPruneOption) (walPruneCallOptions, error) {
	resolved := walPruneCallOptions{}
	for i, option := range options {
		if option == nil {
			return walPruneCallOptions{}, errors.Errorf("wal prune option at index %d is nil", i)
		}
		if err := option.apply(&resolved); err != nil {
			return walPruneCallOptions{}, errors.Wrapf(err, "invalid wal prune option at index %d", i)
		}
	}
	if err := resolved.validate(); err != nil {
		return walPruneCallOptions{}, err
	}
	return resolved, nil
}

func resolveWALPruneCollectionRequest(name string, options []WALPruneOption) (walPruneCollectionRequest, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return walPruneCollectionRequest{}, errors.New("name is required")
	}

	resolved, err := resolveWALPruneCallOptions(options)
	if err != nil {
		return walPruneCollectionRequest{}, err
	}

	return walPruneCollectionRequest{
		Name:               trimmedName,
		TenantID:           resolved.tenantID,
		DatabaseName:       resolved.databaseName,
		DryRun:             resolved.dryRun,
		Vacuum:             resolved.vacuum,
		MaxAgeSeconds:      resolved.maxAgeSeconds,
		MaxBytes:           resolved.maxBytes,
		WatermarkHighBytes: resolved.watermarkHighBytes,
		WatermarkLowBytes:  resolved.watermarkLowBytes,
	}, nil
}

func resolveWALPruneAllRequest(options []WALPruneOption) (walPruneAllRequest, error) {
	resolved, err := resolveWALPruneCallOptions(options)
	if err != nil {
		return walPruneAllRequest{}, err
	}

	return walPruneAllRequest{
		TenantID:           resolved.tenantID,
		DatabaseName:       resolved.databaseName,
		DryRun:             resolved.dryRun,
		Vacuum:             resolved.vacuum,
		MaxAgeSeconds:      resolved.maxAgeSeconds,
		MaxBytes:           resolved.maxBytes,
		WatermarkHighBytes: resolved.watermarkHighBytes,
		WatermarkLowBytes:  resolved.watermarkLowBytes,
	}, nil
}

// PruneCollectionWAL prunes WAL rows for one collection in embedded mode.
func (e *Embedded) PruneCollectionWAL(name string, options ...WALPruneOption) (*WALPruneResult, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	request, err := resolveWALPruneCollectionRequest(name, options)
	if err != nil {
		return nil, err
	}
	return e.pruneCollectionWAL(request)
}

func (e *Embedded) pruneCollectionWAL(request walPruneCollectionRequest) (*WALPruneResult, error) {
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

	respPtr := chromaEmbeddedPruneWALCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response WALPruneResult
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode prune collection wal response")
	}
	return &response, nil
}

// PruneAllWAL prunes WAL rows for all collections in embedded mode.
// Per-collection failures are reported in WALPruneResult.Collections[i].Error.
func (e *Embedded) PruneAllWAL(options ...WALPruneOption) (*WALPruneResult, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	request, err := resolveWALPruneAllRequest(options)
	if err != nil {
		return nil, err
	}
	return e.pruneAllWAL(request)
}

func (e *Embedded) pruneAllWAL(request walPruneAllRequest) (*WALPruneResult, error) {
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

	respPtr := chromaEmbeddedPruneWALAll(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response WALPruneResult
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode prune all wal response")
	}
	return &response, nil
}

// PruneCollectionWAL prunes WAL rows for one collection in managed server mode.
// The server is restarted after the operation, even if prune fails.
// The server is unavailable while prune is running.
// Backups, compaction, rebuild, and WAL prune are mutually exclusive and serialized.
// If prune succeeds but cleanup or restart fails, this returns a non-nil WALPruneResult and a non-nil error.
func (s *Server) PruneCollectionWAL(name string, options ...WALPruneOption) (*WALPruneResult, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}

	request, err := resolveWALPruneCollectionRequest(name, options)
	if err != nil {
		return nil, err
	}

	return s.runWALPrune(func(embedded *Embedded) (*WALPruneResult, error) {
		return embedded.pruneCollectionWAL(request)
	})
}

// PruneAllWAL prunes WAL rows for all collections in managed server mode.
// The server is restarted after the operation, even if prune fails.
// The server is unavailable while prune is running.
// Backups, compaction, rebuild, and WAL prune are mutually exclusive and serialized.
// Per-collection failures are reported in WALPruneResult.Collections[i].Error.
// If prune succeeds but cleanup or restart fails, this returns a non-nil WALPruneResult and a non-nil error.
func (s *Server) PruneAllWAL(options ...WALPruneOption) (*WALPruneResult, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}

	request, err := resolveWALPruneAllRequest(options)
	if err != nil {
		return nil, err
	}

	return s.runWALPrune(func(embedded *Embedded) (*WALPruneResult, error) {
		return embedded.pruneAllWAL(request)
	})
}

func (s *Server) runWALPrune(run func(*Embedded) (*WALPruneResult, error)) (*WALPruneResult, error) {
	// Serialize maintenance operations and keep lock ordering stable: backupMu -> stateMu -> ffiMu.
	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	config, _, err := s.snapshotBackupInputs()
	if err != nil {
		return nil, err
	}
	if err := s.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to stop server before wal prune")
	}

	//nolint:staticcheck // S1016: keep explicit field mapping so server and embedded config types can evolve independently.
	embedded, startErr := StartEmbedded(StartEmbeddedConfig{
		ConfigPath:   config.ConfigPath,
		ConfigString: config.ConfigString,
	})
	if startErr != nil {
		restartErr := s.restartFromConfig(config)
		if restartErr != nil {
			return nil, fmt.Errorf("failed to start temporary embedded runtime for wal prune: %w; restart failed: %w; server remains stopped", startErr, restartErr)
		}
		return nil, errors.Wrap(startErr, "failed to start temporary embedded runtime for wal prune")
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
			return result, fmt.Errorf("wal prune completed but temporary embedded runtime close failed: %w; restart failed: %w; server remains stopped", closeErr, restartErr)
		}
		return result, errors.Wrap(closeErr, "wal prune completed but failed to close temporary embedded runtime")
	case restartErr != nil:
		return result, errors.Wrap(restartErr, "wal prune completed but server restart failed; server remains stopped")
	default:
		return result, nil
	}
}
