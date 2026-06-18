package chroma

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
)

// RebuildCollectionResult reports rebuild outcome, counts, timing, and backup details.
type RebuildCollectionResult struct {
	CollectionID     string   `json:"collection_id"`
	Name             string   `json:"name"`
	TenantID         string   `json:"tenant_id"`
	DatabaseName     string   `json:"database_name"`
	Precheck         bool     `json:"precheck"`
	WouldRebuild     bool     `json:"would_rebuild"`
	Rebuilt          bool     `json:"rebuilt"`
	RecordsScanned   uint64   `json:"records_scanned"`
	VectorsReindexed uint64   `json:"vectors_reindexed"`
	DurationMS       uint64   `json:"duration_ms"`
	BackupPath       string   `json:"backup_path"`
	Warnings         []string `json:"warnings"`
}

type rebuildCollectionRequest struct {
	Name         string `json:"name"`
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
	Precheck     bool   `json:"precheck,omitempty"`
	KeepBackup   bool   `json:"keep_backup"`
}

// RebuildCollectionOption applies optional rebuild scope and behavior.
type RebuildCollectionOption interface {
	apply(*rebuildCollectionCallOptions) error
}

type rebuildCollectionOptionFunc func(*rebuildCollectionCallOptions) error

func (f rebuildCollectionOptionFunc) apply(options *rebuildCollectionCallOptions) error {
	return f(options)
}

type rebuildCollectionCallOptions struct {
	tenantID     string
	databaseName string
	precheck     bool
	keepBackup   bool
}

// WithRebuildTenantID scopes collection lookup to a tenant.
func WithRebuildTenantID(tenantID string) RebuildCollectionOption {
	return rebuildCollectionOptionFunc(func(options *rebuildCollectionCallOptions) error {
		options.tenantID = strings.TrimSpace(tenantID)
		return nil
	})
}

// WithRebuildDatabaseName scopes collection lookup to a database.
func WithRebuildDatabaseName(databaseName string) RebuildCollectionOption {
	return rebuildCollectionOptionFunc(func(options *rebuildCollectionCallOptions) error {
		options.databaseName = strings.TrimSpace(databaseName)
		return nil
	})
}

// WithRebuildPrecheck enables prerequisites-only validation with no mutations.
func WithRebuildPrecheck() RebuildCollectionOption {
	return rebuildCollectionOptionFunc(func(options *rebuildCollectionCallOptions) error {
		options.precheck = true
		return nil
	})
}

// WithRebuildKeepBackup controls whether pre-swap index artifacts are retained.
func WithRebuildKeepBackup(keepBackup bool) RebuildCollectionOption {
	return rebuildCollectionOptionFunc(func(options *rebuildCollectionCallOptions) error {
		options.keepBackup = keepBackup
		return nil
	})
}

func resolveRebuildCollectionRequest(name string, options []RebuildCollectionOption) (rebuildCollectionRequest, error) {
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return rebuildCollectionRequest{}, errors.New("name is required")
	}

	resolved := rebuildCollectionCallOptions{keepBackup: true}
	for i, option := range options {
		if option == nil {
			return rebuildCollectionRequest{}, errors.Errorf("rebuild option at index %d is nil", i)
		}
		if err := option.apply(&resolved); err != nil {
			return rebuildCollectionRequest{}, errors.Wrapf(err, "invalid rebuild option at index %d", i)
		}
	}

	databaseName := strings.TrimSpace(resolved.databaseName)
	if databaseName != "" && len(databaseName) < 3 {
		return rebuildCollectionRequest{}, errors.New("database_name must be at least 3 characters")
	}
	tenantID := strings.TrimSpace(resolved.tenantID)
	if tenantID != "" && len(tenantID) < 3 {
		return rebuildCollectionRequest{}, errors.New("tenant_id must be at least 3 characters")
	}

	return rebuildCollectionRequest{
		Name:         trimmedName,
		TenantID:     tenantID,
		DatabaseName: databaseName,
		Precheck:     resolved.precheck,
		KeepBackup:   resolved.keepBackup,
	}, nil
}

// RebuildCollection runs rebuild in embedded mode for a single collection.
func (e *Embedded) RebuildCollection(name string, options ...RebuildCollectionOption) (*RebuildCollectionResult, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	request, err := resolveRebuildCollectionRequest(name, options)
	if err != nil {
		return nil, err
	}

	return e.rebuildCollection(request)
}

func (e *Embedded) rebuildCollection(request rebuildCollectionRequest) (*RebuildCollectionResult, error) {
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

	respPtr := chromaEmbeddedRebuildCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response RebuildCollectionResult
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode rebuild collection response")
	}
	return &response, nil
}

// RebuildCollection rebuilds index artifacts for one collection in managed server mode.
// After the operation (success or failure), a server restart is attempted.
// The server is unavailable while rebuild is running.
// Backups, compaction, and rebuild are mutually exclusive and serialized.
// If rebuild succeeds but temporary runtime close or restart fails, this returns a non-nil RebuildCollectionResult and a non-nil error.
// If restart fails, the server remains stopped.
func (s *Server) RebuildCollection(name string, options ...RebuildCollectionOption) (*RebuildCollectionResult, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}

	request, err := resolveRebuildCollectionRequest(name, options)
	if err != nil {
		return nil, err
	}

	return s.runRebuild(func(embedded *Embedded) (*RebuildCollectionResult, error) {
		return embedded.rebuildCollection(request)
	})
}

func (s *Server) runRebuild(run func(*Embedded) (*RebuildCollectionResult, error)) (*RebuildCollectionResult, error) {
	// Lock ordering across the call tree: backupMu -> stateMu -> ffiMu.
	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	config, _, err := s.snapshotBackupInputs()
	if err != nil {
		return nil, err
	}
	if err := s.Close(); err != nil {
		return nil, errors.Wrap(err, "failed to stop server before rebuild")
	}

	//nolint:staticcheck // S1016: keep explicit field mapping so server and embedded config types can evolve independently.
	embedded, startErr := StartEmbedded(StartEmbeddedConfig{
		ConfigPath:   config.ConfigPath,
		ConfigString: config.ConfigString,
	})
	if startErr != nil {
		restartErr := s.restartFromConfig(config)
		if restartErr != nil {
			return nil, fmt.Errorf("failed to start temporary embedded runtime for rebuild: %w; restart failed: %w; server remains stopped", startErr, restartErr)
		}
		return nil, errors.Wrap(startErr, "failed to start temporary embedded runtime for rebuild")
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
			return result, fmt.Errorf("rebuild completed but temporary embedded runtime close failed: %w; restart failed: %w; server remains stopped", closeErr, restartErr)
		}
		return result, errors.Wrap(closeErr, "rebuild completed but failed to close temporary embedded runtime")
	case restartErr != nil:
		return result, errors.Wrap(restartErr, "rebuild completed but server restart failed; server remains stopped")
	default:
		return result, nil
	}
}
