package chroma

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
)

const (
	backupManifestFilename = "backup_manifest.json"
	backupSnapshotDirname  = "persist"
	backupSchemaVersion    = "v1"
)

// BackupMode identifies the runtime mode that produced a backup.
type BackupMode string

const (
	BackupModeServer   BackupMode = "server"
	BackupModeEmbedded BackupMode = "embedded"
)

// BackupOptions defines shared backup controls.
type BackupOptions struct {
	// DestinationPath is the target directory where backup data is written.
	// The directory must not exist, or it must exist and be empty.
	DestinationPath string
	// IncludeMetadata includes per-file metadata in the generated manifest.
	IncludeMetadata bool
}

// ServerBackupOptions defines backup behavior for managed server mode.
//
// Deprecated: prefer Backup option helpers (`WithDestination`, `WithIncludeMetadata`, `WithLeaveStopped`).
type ServerBackupOptions struct {
	BackupOptions
	// LeaveStopped keeps the server stopped after backup. By default, Backup restarts it.
	LeaveStopped bool
}

func (o ServerBackupOptions) apply(options *backupCallOptions) error {
	if strings.TrimSpace(o.DestinationPath) != "" {
		if err := WithDestination(o.DestinationPath).apply(options); err != nil {
			return err
		}
	}
	if o.IncludeMetadata {
		options.includeMetadata = true
	}
	if o.LeaveStopped {
		options.leaveStopped = true
	}
	return nil
}

// EmbeddedBackupOptions defines backup behavior for embedded mode.
//
// Deprecated: prefer Backup option helpers (`WithDestination`, `WithIncludeMetadata`, `WithLeaveClosed`).
type EmbeddedBackupOptions struct {
	BackupOptions
	// LeaveClosed keeps embedded mode closed after backup. By default, Backup reopens it.
	LeaveClosed bool
}

func (o EmbeddedBackupOptions) apply(options *backupCallOptions) error {
	if strings.TrimSpace(o.DestinationPath) != "" {
		if err := WithDestination(o.DestinationPath).apply(options); err != nil {
			return err
		}
	}
	if o.IncludeMetadata {
		options.includeMetadata = true
	}
	if o.LeaveClosed {
		options.leaveClosed = true
	}
	return nil
}

// BackupOption configures backup behavior.
type BackupOption interface {
	apply(*backupCallOptions) error
}

type backupOptionFunc func(*backupCallOptions) error

func (f backupOptionFunc) apply(options *backupCallOptions) error {
	return f(options)
}

type backupCallOptions struct {
	destinationPath string
	includeMetadata bool
	leaveStopped    bool
	leaveClosed     bool

	destinationSet bool
}

// WithDestination sets the backup destination directory.
func WithDestination(path string) BackupOption {
	return backupOptionFunc(func(options *backupCallOptions) error {
		if options.destinationSet {
			return errors.New("destination path already set")
		}
		if strings.TrimSpace(path) == "" {
			return errors.New("destination_path is required")
		}
		options.destinationPath = path
		options.destinationSet = true
		return nil
	})
}

// WithIncludeMetadata includes per-file metadata in the generated manifest.
func WithIncludeMetadata() BackupOption {
	return backupOptionFunc(func(options *backupCallOptions) error {
		options.includeMetadata = true
		return nil
	})
}

// WithLeaveStopped keeps managed server mode stopped after backup.
func WithLeaveStopped() BackupOption {
	return backupOptionFunc(func(options *backupCallOptions) error {
		options.leaveStopped = true
		return nil
	})
}

// WithLeaveClosed keeps embedded mode closed after backup.
func WithLeaveClosed() BackupOption {
	return backupOptionFunc(func(options *backupCallOptions) error {
		options.leaveClosed = true
		return nil
	})
}

// BackupFileMetadata captures metadata for a copied file.
type BackupFileMetadata struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	// Mode is a parseable POSIX permission string (for example "0644").
	Mode       string    `json:"mode"`
	SHA256     string    `json:"sha256"`
	ModifiedAt time.Time `json:"modified_at"`
}

// BackupManifest describes a completed backup operation.
type BackupManifest struct {
	SchemaVersion   string               `json:"schema_version"`
	Mode            BackupMode           `json:"mode"`
	CreatedAt       time.Time            `json:"created_at"`
	WrapperVersion  string               `json:"wrapper_version"`
	SourcePaths     []string             `json:"source_paths"`
	DestinationPath string               `json:"destination_path"`
	SnapshotPath    string               `json:"snapshot_path"`
	ManifestPath    string               `json:"manifest_path"`
	IncludeMetadata bool                 `json:"include_metadata"`
	FileCount       int                  `json:"file_count"`
	TotalBytes      int64                `json:"total_bytes"`
	Files           []BackupFileMetadata `json:"files,omitempty"`
}

type backupPlan struct {
	sourcePersistPath string
	sourcePathExists  bool
	sourcePaths       []string
	destinationPath   string
	includeMetadata   bool
	wrapperVersion    string
}

func resolveBackupOptions(mode BackupMode, options []BackupOption) (BackupOptions, bool, error) {
	resolved := backupCallOptions{}
	for i, option := range options {
		if option == nil {
			return BackupOptions{}, false, errors.Errorf("backup option at index %d is nil", i)
		}
		if err := option.apply(&resolved); err != nil {
			return BackupOptions{}, false, errors.Wrapf(err, "invalid backup option at index %d", i)
		}
	}

	if !resolved.destinationSet {
		return BackupOptions{}, false, errors.New("destination_path is required")
	}

	backupOptions := BackupOptions{
		DestinationPath: resolved.destinationPath,
		IncludeMetadata: resolved.includeMetadata,
	}
	switch mode {
	case BackupModeServer:
		if resolved.leaveClosed {
			return BackupOptions{}, false, errors.New("WithLeaveClosed is only valid for embedded backups")
		}
		return backupOptions, resolved.leaveStopped, nil
	case BackupModeEmbedded:
		if resolved.leaveStopped {
			return BackupOptions{}, false, errors.New("WithLeaveStopped is only valid for server backups")
		}
		return backupOptions, resolved.leaveClosed, nil
	default:
		return BackupOptions{}, false, errors.Errorf("unsupported backup mode %q", mode)
	}
}

// Backup closes the server, snapshots its persistence directory, writes a manifest,
// and restarts the server unless WithLeaveStopped is provided.
func (s *Server) Backup(options ...BackupOption) (*BackupManifest, error) {
	if s == nil {
		return nil, ErrServerNotStarted
	}
	resolvedOptions, leaveStopped, err := resolveBackupOptions(BackupModeServer, options)
	if err != nil {
		return nil, err
	}

	s.backupMu.Lock()
	defer s.backupMu.Unlock()

	config, persistPath, err := s.snapshotBackupInputs()
	if err != nil {
		return nil, err
	}

	plan, err := newBackupPlan(persistPath, config.ConfigPath, resolvedOptions)
	if err != nil {
		return nil, err
	}
	if err := ensureEmptyDir(plan.destinationPath); err != nil {
		return nil, err
	}

	if err := s.Close(); err != nil {
		return nil, err
	}

	manifest, backupErr := executeBackup(BackupModeServer, plan)
	if leaveStopped {
		return manifest, backupErr
	}

	restartErr := s.restartFromConfig(config)
	switch {
	case backupErr != nil && restartErr != nil:
		return nil, fmt.Errorf("%w; restart failed: %v", backupErr, restartErr)
	case backupErr != nil:
		return nil, backupErr
	case restartErr != nil:
		return manifest, errors.Wrap(restartErr, "backup completed but server restart failed")
	default:
		return manifest, nil
	}
}

// Backup closes embedded mode, snapshots its persistence directory, writes a
// manifest, and reopens embedded mode unless WithLeaveClosed is provided.
func (e *Embedded) Backup(options ...BackupOption) (*BackupManifest, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}
	resolvedOptions, leaveClosed, err := resolveBackupOptions(BackupModeEmbedded, options)
	if err != nil {
		return nil, err
	}

	e.backupMu.Lock()
	defer e.backupMu.Unlock()

	config, persistPath, err := e.snapshotBackupInputs()
	if err != nil {
		return nil, err
	}

	plan, err := newBackupPlan(persistPath, config.ConfigPath, resolvedOptions)
	if err != nil {
		return nil, err
	}
	if err := ensureEmptyDir(plan.destinationPath); err != nil {
		return nil, err
	}

	if err := e.Close(); err != nil {
		return nil, err
	}

	manifest, backupErr := executeBackup(BackupModeEmbedded, plan)
	if leaveClosed {
		return manifest, backupErr
	}

	reopenErr := e.reopenFromConfig(config)
	switch {
	case backupErr != nil && reopenErr != nil:
		return nil, fmt.Errorf("%w; reopen failed: %v", backupErr, reopenErr)
	case backupErr != nil:
		return nil, backupErr
	case reopenErr != nil:
		return manifest, errors.Wrap(reopenErr, "backup completed but embedded reopen failed")
	default:
		return manifest, nil
	}
}

func (s *Server) restartFromConfig(config StartServerConfig) error {
	restarted, err := StartServer(config)
	if err != nil {
		return err
	}

	handle := atomic.SwapUintptr(&restarted.handle, 0)
	runtime.SetFinalizer(restarted, nil)
	s.stateMu.Lock()
	s.port = restarted.port
	s.addr = restarted.addr
	s.config = restarted.config
	s.persistPath = restarted.persistPath
	atomic.StoreUintptr(&s.handle, handle)
	s.stateMu.Unlock()
	runtime.KeepAlive(restarted)
	return nil
}

func (e *Embedded) reopenFromConfig(config StartEmbeddedConfig) error {
	restarted, err := StartEmbedded(config)
	if err != nil {
		return err
	}

	handle := atomic.SwapUintptr(&restarted.handle, 0)
	runtime.SetFinalizer(restarted, nil)
	e.stateMu.Lock()
	e.config = restarted.config
	e.persistPath = restarted.persistPath
	atomic.StoreUintptr(&e.handle, handle)
	e.stateMu.Unlock()
	runtime.KeepAlive(restarted)
	return nil
}

func (s *Server) snapshotBackupInputs() (StartServerConfig, string, error) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()

	if atomic.LoadUintptr(&s.handle) == 0 {
		return StartServerConfig{}, "", ErrServerNotStarted
	}
	return s.config, s.persistPath, nil
}

func (e *Embedded) snapshotBackupInputs() (StartEmbeddedConfig, string, error) {
	e.stateMu.RLock()
	defer e.stateMu.RUnlock()

	if atomic.LoadUintptr(&e.handle) == 0 {
		return StartEmbeddedConfig{}, "", ErrEmbeddedNotStarted
	}
	return e.config, e.persistPath, nil
}

func newBackupPlan(persistPath, configPath string, options BackupOptions) (*backupPlan, error) {
	dest := strings.TrimSpace(options.DestinationPath)
	if dest == "" {
		return nil, errors.New("destination_path is required")
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve destination path")
	}

	sourceResolved, err := resolveSourcePersistPath(persistPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to resolve source persist path")
	}
	sourcePathExists, err := inspectSourcePath(sourceResolved)
	if err != nil {
		return nil, err
	}

	snapshotPath := filepath.Join(destAbs, backupSnapshotDirname)
	snapshotResolved, err := resolvePathForContainment(snapshotPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to canonicalize destination snapshot path")
	}
	insideSource, withinErr := isWithinPath(snapshotResolved, sourceResolved)
	if withinErr != nil {
		return nil, errors.Wrap(withinErr, "failed to validate destination path containment")
	}
	if insideSource {
		return nil, errors.Errorf("destination path %q cannot be inside source persist path %q", destAbs, sourceResolved)
	}

	version, err := VersionWithError()
	if err != nil {
		version = "unknown"
	}

	sourcePaths := []string{sourceResolved}
	if configPath != "" {
		configAbs, cfgErr := filepath.Abs(configPath)
		if cfgErr == nil {
			sourcePaths = append(sourcePaths, filepath.Clean(configAbs))
		} else {
			sourcePaths = append(sourcePaths, filepath.Clean(configPath))
		}
	}

	return &backupPlan{
		sourcePersistPath: sourceResolved,
		sourcePathExists:  sourcePathExists,
		sourcePaths:       sourcePaths,
		destinationPath:   filepath.Clean(destAbs),
		includeMetadata:   options.IncludeMetadata,
		wrapperVersion:    version,
	}, nil
}

func inspectSourcePath(sourcePath string) (bool, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "failed to inspect source persist path")
	}
	if !info.IsDir() {
		return false, errors.Errorf("source persist path %q must be a directory", sourcePath)
	}
	return true, nil
}

func executeBackup(mode BackupMode, plan *backupPlan) (*BackupManifest, error) {
	snapshotPath := filepath.Join(plan.destinationPath, backupSnapshotDirname)
	fileCount := 0
	totalBytes := int64(0)
	files := []BackupFileMetadata(nil)
	if plan.sourcePathExists {
		var copyErr error
		fileCount, totalBytes, files, copyErr = copyDirectory(plan.sourcePersistPath, snapshotPath, plan.includeMetadata)
		if copyErr != nil {
			return nil, copyErr
		}
	} else if err := os.MkdirAll(snapshotPath, 0o755); err != nil {
		return nil, errors.Wrap(err, "failed to create backup snapshot directory")
	}

	manifestPath := filepath.Join(plan.destinationPath, backupManifestFilename)
	manifest := &BackupManifest{
		SchemaVersion:   backupSchemaVersion,
		Mode:            mode,
		CreatedAt:       time.Now().UTC(),
		WrapperVersion:  plan.wrapperVersion,
		SourcePaths:     plan.sourcePaths,
		DestinationPath: plan.destinationPath,
		SnapshotPath:    snapshotPath,
		ManifestPath:    manifestPath,
		IncludeMetadata: plan.includeMetadata,
		FileCount:       fileCount,
		TotalBytes:      totalBytes,
		Files:           files,
	}

	if err := writeManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func writeManifest(manifest *BackupManifest) error {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to encode backup manifest")
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(manifest.ManifestPath, payload, 0o644); err != nil {
		return errors.Wrap(err, "failed to write backup manifest")
	}
	return nil
}

func ensureEmptyDir(path string) error {
	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		return os.MkdirAll(path, 0o755)
	case err != nil:
		return errors.Wrap(err, "failed to inspect destination path")
	case !info.IsDir():
		return errors.Errorf("destination path %q must be a directory", path)
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return errors.Wrap(err, "failed to inspect destination directory contents")
	}
	if len(entries) > 0 {
		return errors.Errorf("destination path %q must be empty", path)
	}
	return nil
}

func copyDirectory(sourceDir, destinationDir string, includeMetadata bool) (int, int64, []BackupFileMetadata, error) {
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return 0, 0, nil, errors.Wrap(err, "failed to create backup snapshot directory")
	}

	fileCount := 0
	totalBytes := int64(0)
	files := []BackupFileMetadata(nil)

	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		destPath := filepath.Join(destinationDir, rel)

		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(destPath, info.Mode().Perm()); err != nil {
				return err
			}
			return nil
		}

		if entry.Type()&os.ModeSymlink != 0 {
			return errors.Errorf("backup does not support symbolic links: %q", path)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		checksum, err := copyFile(path, destPath, info)
		if err != nil {
			return err
		}

		fileCount++
		totalBytes += info.Size()
		if includeMetadata {
			files = append(files, BackupFileMetadata{
				Path:       filepath.ToSlash(rel),
				SizeBytes:  info.Size(),
				Mode:       fmt.Sprintf("%#o", uint32(info.Mode().Perm())),
				SHA256:     checksum,
				ModifiedAt: info.ModTime().UTC(),
			})
		}
		return nil
	})
	if err != nil {
		return 0, 0, nil, errors.Wrap(err, "failed to copy persistence directory")
	}

	return fileCount, totalBytes, files, nil
}

func copyFile(sourcePath, destinationPath string, info os.FileInfo) (checksum string, err error) {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return "", err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer func() {
		closeErr := sourceFile.Close()
		if closeErr == nil {
			return
		}
		if err == nil {
			err = errors.Wrap(closeErr, "failed to close source file")
		}
	}()

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return "", err
	}

	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(destinationFile, hash), sourceFile); err != nil {
		_ = destinationFile.Close()
		return "", err
	}
	if err := destinationFile.Sync(); err != nil {
		_ = destinationFile.Close()
		return "", err
	}
	if err := destinationFile.Close(); err != nil {
		return "", err
	}
	if err := os.Chtimes(destinationPath, info.ModTime(), info.ModTime()); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func normalizePersistPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", errors.New("persist path is empty")
	}

	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve persist path")
	}
	return filepath.Clean(absPath), nil
}

func resolveSourcePersistPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", errors.New("persist path is empty")
	}
	absPath, err := filepath.Abs(cleaned)
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve persist path")
	}
	resolved, err := evalSymlinksWithMissingSegments(filepath.Clean(absPath))
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func resolvePathForContainment(path string) (string, error) {
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", errors.Wrap(err, "failed to resolve path")
	}

	resolved, err := evalSymlinksWithMissingSegments(absPath)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

func evalSymlinksWithMissingSegments(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", errors.Wrap(err, "failed to evaluate symlinks")
	}

	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}

	resolvedParent, err := evalSymlinksWithMissingSegments(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func isWithinPath(path, parent string) (bool, error) {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	prefix := ".." + string(filepath.Separator)
	return rel != ".." && !strings.HasPrefix(rel, prefix), nil
}
