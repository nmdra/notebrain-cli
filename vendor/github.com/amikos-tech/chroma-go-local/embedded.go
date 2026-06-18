package chroma

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
)

const (
	// Default tenant and database used by Chroma local mode.
	DefaultTenantID    = "default_tenant"
	DefaultDatabase    = "default_database"
	DefaultEmbeddedDir = "./chroma"
)

// Embedded represents an in-process Chroma frontend (no HTTP server).
type Embedded struct {
	stateMu  sync.RWMutex
	backupMu sync.Mutex

	handle      uintptr
	config      StartEmbeddedConfig
	persistPath string
}

// StartEmbeddedConfig contains configuration options for starting embedded mode.
type StartEmbeddedConfig struct {
	ConfigPath   string // Path to YAML config file
	ConfigString string // YAML config string (used if ConfigPath is empty)
}

// EmbeddedConfig holds simple embedded configuration defaults.
type EmbeddedConfig struct {
	PersistPath    string
	SQLiteFilename string
	AllowReset     bool

	rawYAML string
}

// EmbeddedOption configures EmbeddedConfig.
type EmbeddedOption func(*EmbeddedConfig)

// DefaultEmbeddedConfig returns a default embedded config.
func DefaultEmbeddedConfig() *EmbeddedConfig {
	return &EmbeddedConfig{
		PersistPath:    DefaultEmbeddedDir,
		SQLiteFilename: "chroma.sqlite3",
		AllowReset:     false,
	}
}

// WithEmbeddedPersistPath sets the embedded persistence directory.
func WithEmbeddedPersistPath(path string) EmbeddedOption {
	return func(c *EmbeddedConfig) {
		c.PersistPath = path
	}
}

// WithEmbeddedSQLiteFilename sets the SQLite filename.
func WithEmbeddedSQLiteFilename(filename string) EmbeddedOption {
	return func(c *EmbeddedConfig) {
		c.SQLiteFilename = filename
	}
}

// WithEmbeddedAllowReset enables reset in embedded mode.
func WithEmbeddedAllowReset(allow bool) EmbeddedOption {
	return func(c *EmbeddedConfig) {
		c.AllowReset = allow
	}
}

// WithEmbeddedRawYAML uses a raw YAML config (overrides other options).
func WithEmbeddedRawYAML(yaml string) EmbeddedOption {
	return func(c *EmbeddedConfig) {
		c.rawYAML = yaml
	}
}

func (c *EmbeddedConfig) toYAML() string {
	if c.rawYAML != "" {
		return c.rawYAML
	}

	var b strings.Builder
	fmt.Fprintf(&b, "persist_path: %q\n", c.PersistPath)
	fmt.Fprintf(&b, "sqlite_filename: %q\n", c.SQLiteFilename)
	fmt.Fprintf(&b, "allow_reset: %t\n", c.AllowReset)
	return b.String()
}

// EmbeddedCreateCollectionRequest creates a collection in embedded mode.
type EmbeddedCreateCollectionRequest struct {
	Name          string         `json:"name"`
	TenantID      string         `json:"tenant_id,omitempty"`
	DatabaseName  string         `json:"database_name,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	Configuration map[string]any `json:"configuration,omitempty"`
	Schema        map[string]any `json:"schema,omitempty"`
	GetOrCreate   bool           `json:"get_or_create,omitempty"`
}

// EmbeddedCollection is a compact view of a created collection.
type EmbeddedCollection struct {
	ID                string         `json:"id"`
	Name              string         `json:"name"`
	Tenant            string         `json:"tenant"`
	Database          string         `json:"database"`
	Metadata          map[string]any `json:"metadata"`
	ConfigurationJSON map[string]any `json:"configuration_json"`
	Schema            map[string]any `json:"schema"`
}

// EmbeddedDatabase is a compact view of a database.
type EmbeddedDatabase struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tenant string `json:"tenant"`
}

// EmbeddedTenant is a compact view of a tenant.
type EmbeddedTenant struct {
	Name         string  `json:"name"`
	ResourceName *string `json:"resource_name,omitempty"`
}

// EmbeddedCreateTenantRequest creates a tenant in embedded mode.
type EmbeddedCreateTenantRequest struct {
	Name string `json:"name"`
}

// EmbeddedGetTenantRequest gets a tenant by name.
type EmbeddedGetTenantRequest struct {
	Name string `json:"name"`
}

// EmbeddedUpdateTenantRequest updates tenant properties.
type EmbeddedUpdateTenantRequest struct {
	TenantID     string `json:"tenant_id"`
	ResourceName string `json:"resource_name"`
}

// EmbeddedCreateDatabaseRequest creates a database in embedded mode.
type EmbeddedCreateDatabaseRequest struct {
	Name     string `json:"name"`
	TenantID string `json:"tenant_id,omitempty"`
}

// EmbeddedListDatabasesRequest lists databases in embedded mode.
type EmbeddedListDatabasesRequest struct {
	TenantID string `json:"tenant_id,omitempty"`
	Limit    uint32 `json:"limit,omitempty"`
	Offset   uint32 `json:"offset,omitempty"`
}

// EmbeddedGetDatabaseRequest gets a single database.
type EmbeddedGetDatabaseRequest struct {
	Name     string `json:"name"`
	TenantID string `json:"tenant_id,omitempty"`
}

// EmbeddedDeleteDatabaseRequest deletes a single database.
type EmbeddedDeleteDatabaseRequest struct {
	Name     string `json:"name"`
	TenantID string `json:"tenant_id,omitempty"`
}

// EmbeddedListCollectionsRequest lists collections for a database.
type EmbeddedListCollectionsRequest struct {
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
	Limit        uint32 `json:"limit,omitempty"`
	Offset       uint32 `json:"offset,omitempty"`
}

// EmbeddedGetCollectionRequest gets a collection by name.
type EmbeddedGetCollectionRequest struct {
	Name         string `json:"name"`
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// EmbeddedCountCollectionsRequest counts collections for a database.
type EmbeddedCountCollectionsRequest struct {
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// EmbeddedUpdateCollectionRequest updates collection properties.
// NewMetadata replaces the entire existing collection metadata
// (keys not present in NewMetadata are removed; nil values are not permitted).
type EmbeddedUpdateCollectionRequest struct {
	CollectionID string         `json:"collection_id"`
	NewName      string         `json:"new_name,omitempty"`
	NewMetadata  map[string]any `json:"new_metadata,omitempty"`
	DatabaseName string         `json:"database_name,omitempty"`
}

// EmbeddedDeleteCollectionRequest deletes a collection by name.
type EmbeddedDeleteCollectionRequest struct {
	Name         string `json:"name"`
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// EmbeddedForkCollectionRequest forks a source collection into a new target collection.
type EmbeddedForkCollectionRequest struct {
	SourceCollectionID   string `json:"source_collection_id"`
	TargetCollectionName string `json:"target_collection_name"`
	TenantID             string `json:"tenant_id,omitempty"`
	DatabaseName         string `json:"database_name,omitempty"`
}

// EmbeddedAddRequest adds records to a collection.
type EmbeddedAddRequest struct {
	CollectionID string      `json:"collection_id"`
	IDs          []string    `json:"ids"`
	Embeddings   [][]float32 `json:"embeddings"`
	Documents    []string    `json:"documents,omitempty"`
	URIs         []string    `json:"uris,omitempty"`
	// Metadatas accepts bool/int/float/string values and homogeneous arrays of those scalar types.
	// Floats are encoded with an explicit decimal to avoid accidental int-array coercion.
	Metadatas    []map[string]any `json:"metadatas,omitempty"`
	TenantID     string           `json:"tenant_id,omitempty"`
	DatabaseName string           `json:"database_name,omitempty"`
}

// EmbeddedQueryRequest queries vectors from a collection.
type EmbeddedQueryRequest struct {
	CollectionID    string         `json:"collection_id"`
	QueryEmbeddings [][]float32    `json:"query_embeddings"`
	NResults        uint32         `json:"n_results,omitempty"`
	IDs             []string       `json:"ids,omitempty"`
	Where           map[string]any `json:"where,omitempty"`
	WhereDocument   map[string]any `json:"where_document,omitempty"`
	Include         []string       `json:"include,omitempty"`
	TenantID        string         `json:"tenant_id,omitempty"`
	DatabaseName    string         `json:"database_name,omitempty"`
}

// EmbeddedQueryResponse contains top match ids per query embedding.
type EmbeddedQueryResponse struct {
	IDs [][]string `json:"ids"`
}

// EmbeddedCountRecordsRequest counts records in a collection.
type EmbeddedCountRecordsRequest struct {
	CollectionID string `json:"collection_id"`
	TenantID     string `json:"tenant_id,omitempty"`
	DatabaseName string `json:"database_name,omitempty"`
}

// EmbeddedGetRecordsRequest fetches records by ids, filters, or pagination.
type EmbeddedGetRecordsRequest struct {
	CollectionID  string         `json:"collection_id"`
	IDs           []string       `json:"ids,omitempty"`
	Where         map[string]any `json:"where,omitempty"`
	WhereDocument map[string]any `json:"where_document,omitempty"`
	Limit         uint32         `json:"limit,omitempty"`
	Offset        uint32         `json:"offset,omitempty"`
	Include       []string       `json:"include,omitempty"`
	TenantID      string         `json:"tenant_id,omitempty"`
	DatabaseName  string         `json:"database_name,omitempty"`
}

// EmbeddedGetRecordsResponse contains fetched record fields.
type EmbeddedGetRecordsResponse struct {
	IDs        []string    `json:"ids"`
	Embeddings [][]float32 `json:"embeddings,omitempty"`
	Documents  []*string   `json:"documents,omitempty"`
	URIs       []*string   `json:"uris,omitempty"`
	// Metadatas decodes through encoding/json into map[string]any.
	// Numeric values (including integer metadata) round-trip back as float64.
	Metadatas []map[string]any `json:"metadatas,omitempty"`
	Include   []string         `json:"include,omitempty"`
}

// EmbeddedUpdateRecordsRequest updates existing records by id.
type EmbeddedUpdateRecordsRequest struct {
	CollectionID string      `json:"collection_id"`
	IDs          []string    `json:"ids"`
	Embeddings   [][]float32 `json:"embeddings,omitempty"`
	Documents    []string    `json:"documents,omitempty"`
	URIs         []string    `json:"uris,omitempty"`
	// Metadatas accepts bool/int/float/string values and homogeneous arrays of those scalar types.
	// Floats are encoded with an explicit decimal to avoid accidental int-array coercion.
	// Nil metadata values are allowed in update/upsert and forwarded as metadata key deletion.
	Metadatas    []map[string]any `json:"metadatas,omitempty"`
	TenantID     string           `json:"tenant_id,omitempty"`
	DatabaseName string           `json:"database_name,omitempty"`
}

// EmbeddedUpsertRecordsRequest upserts records by id.
type EmbeddedUpsertRecordsRequest struct {
	CollectionID string      `json:"collection_id"`
	IDs          []string    `json:"ids"`
	Embeddings   [][]float32 `json:"embeddings"`
	Documents    []string    `json:"documents,omitempty"`
	URIs         []string    `json:"uris,omitempty"`
	// Metadatas accepts bool/int/float/string values and homogeneous arrays of those scalar types.
	// Floats are encoded with an explicit decimal to avoid accidental int-array coercion.
	// Nil metadata values are allowed in update/upsert and forwarded as metadata key deletion.
	Metadatas    []map[string]any `json:"metadatas,omitempty"`
	TenantID     string           `json:"tenant_id,omitempty"`
	DatabaseName string           `json:"database_name,omitempty"`
}

// EmbeddedDeleteRecordsRequest deletes records by ids and/or filters.
type EmbeddedDeleteRecordsRequest struct {
	CollectionID  string         `json:"collection_id"`
	IDs           []string       `json:"ids,omitempty"`
	Where         map[string]any `json:"where,omitempty"`
	WhereDocument map[string]any `json:"where_document,omitempty"`
	// Limit caps filtered deletes. It must be greater than zero and requires
	// Where or WhereDocument. Nil means no limit.
	// Deletion order depends on upstream Chroma internals and may change across versions.
	Limit        *uint32 `json:"limit,omitempty"`
	TenantID     string  `json:"tenant_id,omitempty"`
	DatabaseName string  `json:"database_name,omitempty"`
}

// EmbeddedDeleteRecordsResponse reports how many records were deleted.
type EmbeddedDeleteRecordsResponse struct {
	Deleted uint32 `json:"deleted"`
}

const (
	deleteRecordsLimitRequiresFilterErr = "limit can only be specified when a where or where_document clause is provided"
	deleteRecordsLimitMustBePositiveErr = "limit must be greater than 0"
)

// EmbeddedIndexingStatusRequest gets indexing progress for a collection.
type EmbeddedIndexingStatusRequest struct {
	CollectionID string `json:"collection_id"`
	DatabaseName string `json:"database_name,omitempty"`
}

// EmbeddedIndexingStatusResponse describes indexing progress in local mode.
type EmbeddedIndexingStatusResponse struct {
	OpIndexingProgress float32 `json:"op_indexing_progress"`
	NumUnindexedOps    uint64  `json:"num_unindexed_ops"`
	NumIndexedOps      uint64  `json:"num_indexed_ops"`
	TotalOps           uint64  `json:"total_ops"`
}

// EmbeddedHealthCheckResponse describes embedded readiness state.
type EmbeddedHealthCheckResponse struct {
	IsExecutorReady  bool `json:"is_executor_ready"`
	IsLogClientReady bool `json:"is_log_client_ready"`
}

// NewEmbedded starts embedded mode with builder options.
func NewEmbedded(opts ...EmbeddedOption) (*Embedded, error) {
	cfg := DefaultEmbeddedConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	return StartEmbedded(StartEmbeddedConfig{ConfigString: cfg.toYAML()})
}

// StartEmbedded starts in-process embedded mode.
func StartEmbedded(config StartEmbeddedConfig) (*Embedded, error) {
	if libHandle == 0 {
		return nil, ErrLibraryNotLoaded
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()

	var handle uintptr
	switch {
	case config.ConfigPath != "":
		pathBytes := cStringFromGo(config.ConfigPath)
		handle = chromaEmbeddedStart(&pathBytes[0])
	case config.ConfigString != "":
		yamlBytes := cStringFromGo(config.ConfigString)
		handle = chromaEmbeddedStartFromString(&yamlBytes[0])
	default:
		return nil, errors.New("either ConfigPath or ConfigString must be provided")
	}

	if handle == 0 {
		return nil, nullPointerError(getLastErrorUnlocked())
	}

	persistPathPtr := chromaEmbeddedPersistPath(handle)
	persistPath := ""
	if persistPathPtr != nil {
		persistPath = goStringFromPtr(persistPathPtr)
	}
	resolvedPersistPath, persistPathErr := normalizePersistPath(persistPath)
	if persistPathErr != nil {
		chromaEmbeddedFree(handle)
		return nil, errors.Wrap(persistPathErr, "failed to resolve persist path from runtime config")
	}

	embedded := &Embedded{
		handle:      handle,
		config:      config,
		persistPath: resolvedPersistPath,
	}
	runtime.SetFinalizer(embedded, func(e *Embedded) {
		_ = e.Close()
	})
	return embedded, nil
}

// Heartbeat returns unix nanoseconds from in-process frontend heartbeat.
func (e *Embedded) Heartbeat() (uint64, error) {
	if e == nil {
		return 0, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return 0, ErrEmbeddedNotStarted
	}

	var heartbeat uint64
	rc := chromaEmbeddedHeartbeat(handle, &heartbeat)
	if rc != Success {
		return 0, errorFromCode(rc, getLastErrorUnlocked())
	}
	return heartbeat, nil
}

// MaxBatchSize returns the configured max batch size.
func (e *Embedded) MaxBatchSize() (uint32, error) {
	if e == nil {
		return 0, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return 0, ErrEmbeddedNotStarted
	}

	var maxBatchSize uint32
	rc := chromaEmbeddedGetMaxBatchSize(handle, &maxBatchSize)
	if rc != Success {
		return 0, errorFromCode(rc, getLastErrorUnlocked())
	}
	return maxBatchSize, nil
}

// CreateTenant creates a tenant.
func (e *Embedded) CreateTenant(request EmbeddedCreateTenantRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedCreateTenant(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// GetTenant gets a tenant by name.
func (e *Embedded) GetTenant(request EmbeddedGetTenantRequest) (*EmbeddedTenant, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedGetTenant(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var tenant EmbeddedTenant
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &tenant); err != nil {
		return nil, errors.Wrap(err, "failed to decode get tenant response")
	}
	return &tenant, nil
}

// UpdateTenant updates tenant properties.
func (e *Embedded) UpdateTenant(request EmbeddedUpdateTenantRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if strings.TrimSpace(request.ResourceName) == "" {
		return errors.New("resource_name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedUpdateTenant(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// Healthcheck returns local readiness of internal embedded components.
func (e *Embedded) Healthcheck() (*EmbeddedHealthCheckResponse, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}

	respPtr := chromaEmbeddedHealthcheck(handle)
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response EmbeddedHealthCheckResponse
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode healthcheck response")
	}
	return &response, nil
}

// IndexingStatus reports indexing progress for a collection.
func (e *Embedded) IndexingStatus(request EmbeddedIndexingStatusRequest) (*EmbeddedIndexingStatusResponse, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return nil, errors.New("collection_id is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedIndexingStatus(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response EmbeddedIndexingStatusResponse
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode indexing status response")
	}
	return &response, nil
}

// Reset resets local state if allow_reset is enabled.
func (e *Embedded) Reset() error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}

	rc := chromaEmbeddedReset(handle)
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// CreateDatabase creates a database.
func (e *Embedded) CreateDatabase(request EmbeddedCreateDatabaseRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedCreateDatabase(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// ListDatabases lists databases.
func (e *Embedded) ListDatabases(request EmbeddedListDatabasesRequest) ([]EmbeddedDatabase, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
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

	respPtr := chromaEmbeddedListDatabases(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var databases []EmbeddedDatabase
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &databases); err != nil {
		return nil, errors.Wrap(err, "failed to decode list databases response")
	}
	return databases, nil
}

// GetDatabase gets a database by name.
func (e *Embedded) GetDatabase(request EmbeddedGetDatabaseRequest) (*EmbeddedDatabase, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedGetDatabase(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var database EmbeddedDatabase
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &database); err != nil {
		return nil, errors.Wrap(err, "failed to decode get database response")
	}
	return &database, nil
}

// DeleteDatabase deletes a database by name.
func (e *Embedded) DeleteDatabase(request EmbeddedDeleteDatabaseRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedDeleteDatabase(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// ListCollections lists collections for a database.
func (e *Embedded) ListCollections(request EmbeddedListCollectionsRequest) ([]EmbeddedCollection, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
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

	respPtr := chromaEmbeddedListCollections(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var collections []EmbeddedCollection
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &collections); err != nil {
		return nil, errors.Wrap(err, "failed to decode list collections response")
	}
	return collections, nil
}

// GetCollection gets a collection by name.
func (e *Embedded) GetCollection(request EmbeddedGetCollectionRequest) (*EmbeddedCollection, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedGetCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var collection EmbeddedCollection
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &collection); err != nil {
		return nil, errors.Wrap(err, "failed to decode get collection response")
	}
	return &collection, nil
}

// CountCollections counts collections for a database.
func (e *Embedded) CountCollections(request EmbeddedCountCollectionsRequest) (uint32, error) {
	if e == nil {
		return 0, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return 0, ErrEmbeddedNotStarted
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return 0, err
	}

	var count uint32
	rc := chromaEmbeddedCountCollections(handle, &requestBytes[0], &count)
	if rc != Success {
		return 0, errorFromCode(rc, getLastErrorUnlocked())
	}
	return count, nil
}

// UpdateCollection updates collection properties.
func (e *Embedded) UpdateCollection(request EmbeddedUpdateCollectionRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return errors.New("collection_id is required")
	}
	hasNewName := strings.TrimSpace(request.NewName) != ""
	hasNewMetadata := request.NewMetadata != nil
	if !hasNewName && !hasNewMetadata {
		return errors.New("at least one of new_name or new_metadata is required")
	}
	if hasNewMetadata && len(request.NewMetadata) == 0 {
		return errors.New("new_metadata must not be empty when provided")
	}

	requestPayload := request
	if hasNewMetadata {
		normalizedMetadata, err := validateAndNormalizeMetadata(request.NewMetadata, false)
		if err != nil {
			return errors.Wrap(err, "invalid new_metadata")
		}
		requestPayload.NewMetadata = normalizedMetadata
	}

	requestBytes, err := marshalRequestJSON(requestPayload)
	if err != nil {
		return errors.Wrap(err, "update collection request")
	}

	rc := chromaEmbeddedUpdateCollection(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// DeleteCollection deletes a collection by name.
func (e *Embedded) DeleteCollection(request EmbeddedDeleteCollectionRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedDeleteCollection(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// ForkCollection forks a source collection into a target collection.
func (e *Embedded) ForkCollection(request EmbeddedForkCollectionRequest) (*EmbeddedCollection, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.SourceCollectionID) == "" {
		return nil, errors.New("source_collection_id is required")
	}
	if strings.TrimSpace(request.TargetCollectionName) == "" {
		return nil, errors.New("target_collection_name is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedForkCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var collection EmbeddedCollection
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &collection); err != nil {
		return nil, errors.Wrap(err, "failed to decode fork collection response")
	}
	return &collection, nil
}

// CountRecords counts records in a collection.
func (e *Embedded) CountRecords(request EmbeddedCountRecordsRequest) (uint32, error) {
	if e == nil {
		return 0, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return 0, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return 0, errors.New("collection_id is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return 0, err
	}

	var count uint32
	rc := chromaEmbeddedCount(handle, &requestBytes[0], &count)
	if rc != Success {
		return 0, errorFromCode(rc, getLastErrorUnlocked())
	}
	return count, nil
}

// GetRecords fetches records from a collection.
func (e *Embedded) GetRecords(request EmbeddedGetRecordsRequest) (*EmbeddedGetRecordsResponse, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return nil, errors.New("collection_id is required")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedGet(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response EmbeddedGetRecordsResponse
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode get records response")
	}
	return &response, nil
}

// UpdateRecords updates existing records by id.
func (e *Embedded) UpdateRecords(request EmbeddedUpdateRecordsRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return errors.New("collection_id is required")
	}
	if len(request.IDs) == 0 {
		return errors.New("ids must not be empty")
	}
	if err := validateOptionalLength("embeddings", len(request.Embeddings), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("documents", len(request.Documents), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("uris", len(request.URIs), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("metadatas", len(request.Metadatas), len(request.IDs)); err != nil {
		return err
	}
	if len(request.Embeddings) == 0 && len(request.Documents) == 0 && len(request.URIs) == 0 && len(request.Metadatas) == 0 {
		return errors.New("at least one of embeddings, documents, uris, or metadatas must be provided")
	}
	normalizedMetadatas, err := validateAndNormalizeMetadatas(request.Metadatas, true)
	if err != nil {
		return errors.Wrap(err, "invalid metadatas")
	}
	request.Metadatas = normalizedMetadatas

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedUpdate(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// UpsertRecords upserts records by id.
func (e *Embedded) UpsertRecords(request EmbeddedUpsertRecordsRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return errors.New("collection_id is required")
	}
	if len(request.IDs) == 0 {
		return errors.New("ids must not be empty")
	}
	if len(request.Embeddings) == 0 {
		return errors.New("embeddings must not be empty")
	}
	if len(request.IDs) != len(request.Embeddings) {
		return errors.New("ids and embeddings must have same length")
	}
	if err := validateOptionalLength("documents", len(request.Documents), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("uris", len(request.URIs), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("metadatas", len(request.Metadatas), len(request.IDs)); err != nil {
		return err
	}
	normalizedMetadatas, err := validateAndNormalizeMetadatas(request.Metadatas, true)
	if err != nil {
		return errors.Wrap(err, "invalid metadatas")
	}
	request.Metadatas = normalizedMetadatas

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedUpsert(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

func validateDeleteRecordsRequest(request EmbeddedDeleteRecordsRequest) error {
	if strings.TrimSpace(request.CollectionID) == "" {
		return errors.New("collection_id is required")
	}
	if len(request.IDs) == 0 && len(request.Where) == 0 && len(request.WhereDocument) == 0 {
		return errors.New("at least one of ids, where, or where_document must be provided")
	}
	if request.Limit != nil {
		if *request.Limit == 0 {
			return errors.New(deleteRecordsLimitMustBePositiveErr)
		}
		if len(request.Where) == 0 && len(request.WhereDocument) == 0 {
			return errors.New(deleteRecordsLimitRequiresFilterErr)
		}
	}
	return nil
}

// DeleteRecords deletes records by ids and/or filters.
// The deleted-count response is discarded to preserve the original API.
func (e *Embedded) DeleteRecords(request EmbeddedDeleteRecordsRequest) error {
	_, err := e.DeleteRecordsWithResponse(request)
	return err
}

// DeleteRecordsWithResponse deletes records by ids and/or filters and returns the delete count.
func (e *Embedded) DeleteRecordsWithResponse(request EmbeddedDeleteRecordsRequest) (*EmbeddedDeleteRecordsResponse, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if err := validateDeleteRecordsRequest(request); err != nil {
		return nil, err
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedDeleteRecordsWithResponse(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response EmbeddedDeleteRecordsResponse
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode delete records response")
	}
	return &response, nil
}

// CreateCollection creates a collection and returns a compact response object.
func (e *Embedded) CreateCollection(request EmbeddedCreateCollectionRequest) (*EmbeddedCollection, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.Name) == "" {
		return nil, errors.New("name is required")
	}
	normalizedMetadata, err := validateAndNormalizeMetadata(request.Metadata, false)
	if err != nil {
		return nil, errors.Wrap(err, "invalid metadata")
	}
	request.Metadata = normalizedMetadata

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, errors.Wrap(err, "create collection request")
	}

	respPtr := chromaEmbeddedCreateCollection(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var collection EmbeddedCollection
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &collection); err != nil {
		return nil, errors.Wrap(err, "failed to decode collection response")
	}
	return &collection, nil
}

// Add adds records into an existing collection.
func (e *Embedded) Add(request EmbeddedAddRequest) error {
	if e == nil {
		return ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return errors.New("collection_id is required")
	}
	if len(request.IDs) == 0 {
		return errors.New("ids must not be empty")
	}
	if len(request.Embeddings) == 0 {
		return errors.New("embeddings must not be empty")
	}
	if len(request.IDs) != len(request.Embeddings) {
		return errors.New("ids and embeddings must have same length")
	}
	if err := validateOptionalLength("documents", len(request.Documents), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("uris", len(request.URIs), len(request.IDs)); err != nil {
		return err
	}
	if err := validateOptionalLength("metadatas", len(request.Metadatas), len(request.IDs)); err != nil {
		return err
	}
	normalizedMetadatas, err := validateAndNormalizeMetadatas(request.Metadatas, false)
	if err != nil {
		return errors.Wrap(err, "invalid metadatas")
	}
	request.Metadatas = normalizedMetadatas

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return err
	}

	rc := chromaEmbeddedAdd(handle, &requestBytes[0])
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// Query runs nearest-neighbor search against a collection.
func (e *Embedded) Query(request EmbeddedQueryRequest) (*EmbeddedQueryResponse, error) {
	if e == nil {
		return nil, ErrEmbeddedNotStarted
	}

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)
	handle := atomic.LoadUintptr(&e.handle)
	if handle == 0 {
		return nil, ErrEmbeddedNotStarted
	}
	if strings.TrimSpace(request.CollectionID) == "" {
		return nil, errors.New("collection_id is required")
	}
	if len(request.QueryEmbeddings) == 0 {
		return nil, errors.New("query_embeddings must not be empty")
	}

	requestBytes, err := marshalRequestJSON(request)
	if err != nil {
		return nil, err
	}

	respPtr := chromaEmbeddedQuery(handle, &requestBytes[0])
	if respPtr == nil {
		return nil, errors.Wrap(ErrNullPointer, getLastErrorUnlocked())
	}
	defer chromaStringFree(respPtr)

	var response EmbeddedQueryResponse
	if err := json.Unmarshal([]byte(goStringFromPtr(respPtr)), &response); err != nil {
		return nil, errors.Wrap(err, "failed to decode query response")
	}
	return &response, nil
}

// Close releases embedded mode resources.
func (e *Embedded) Close() error {
	if e == nil {
		return nil
	}
	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(e)

	handle := atomic.SwapUintptr(&e.handle, 0)
	if handle == 0 {
		return nil
	}

	chromaEmbeddedFree(handle)
	return nil
}

func validateOptionalLength(field string, valueLen, idsLen int) error {
	if valueLen > 0 && valueLen != idsLen {
		return errors.Errorf("%s must have same length as ids when provided", field)
	}
	return nil
}

func validateAndNormalizeMetadata(metadata map[string]any, allowNilValues bool) (map[string]any, error) {
	if len(metadata) == 0 {
		return metadata, nil
	}

	normalizedMetadata := make(map[string]any, len(metadata))
	for key, value := range metadata {
		path := fmt.Sprintf("metadata.%s", key)
		normalizedValue, err := normalizeMetadataValue(path, value, allowNilValues)
		if err != nil {
			return nil, err
		}
		normalizedMetadata[key] = normalizedValue
	}
	return normalizedMetadata, nil
}

// metadataFloat64 preserves explicit floating-point representation in JSON.
// This avoids ambiguous encoding of whole floats (for example 1.0 -> 1).
type metadataFloat64 float64

func (f metadataFloat64) MarshalJSON() ([]byte, error) {
	v := float64(f)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errors.New("float metadata values must be finite")
	}
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return []byte(s), nil
}

func validateAndNormalizeMetadatas(metadatas []map[string]any, allowNilValues bool) ([]map[string]any, error) {
	if len(metadatas) == 0 {
		return metadatas, nil
	}

	normalized := make([]map[string]any, len(metadatas))
	for i, metadata := range metadatas {
		if metadata == nil {
			normalized[i] = nil
			continue
		}

		normalizedMetadata := make(map[string]any, len(metadata))
		for key, value := range metadata {
			path := fmt.Sprintf("metadatas[%d].%s", i, key)
			normalizedValue, err := normalizeMetadataValue(path, value, allowNilValues)
			if err != nil {
				return nil, err
			}
			normalizedMetadata[key] = normalizedValue
		}
		normalized[i] = normalizedMetadata
	}

	return normalized, nil
}

func normalizeMetadataValue(path string, value any, allowNil bool) (any, error) {
	if value == nil {
		if allowNil {
			return nil, nil
		}
		return nil, errors.Errorf("%s cannot be null", path)
	}

	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Bool:
		return rv.Bool(), nil
	case reflect.String:
		return rv.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u > uint64(^uint64(0)>>1) {
			return nil, errors.Errorf("%s integer value %d exceeds int64 range", path, u)
		}
		return int64(u), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, errors.Errorf("%s float metadata values must be finite", path)
		}
		return metadataFloat64(f), nil
	case reflect.Ptr:
		if rv.IsNil() {
			if allowNil {
				return nil, nil
			}
			return nil, errors.Errorf("%s cannot be null", path)
		}
		return normalizeMetadataValue(path, rv.Elem().Interface(), allowNil)
	case reflect.Slice, reflect.Array:
		return normalizeMetadataSlice(path, rv)
	case reflect.Map:
		return nil, errors.Errorf("%s has unsupported metadata value type %T (nested objects are not supported)", path, value)
	default:
		return nil, errors.Errorf("%s has unsupported metadata value type %T", path, value)
	}
}

func normalizeMetadataSlice(path string, rv reflect.Value) (any, error) {
	if rv.Type().Elem().Kind() == reflect.Uint8 {
		return nil, errors.Errorf("%s has unsupported metadata array type %s", path, rv.Type().String())
	}

	if rv.Len() == 0 {
		switch rv.Type().Elem().Kind() {
		case reflect.Bool:
			return []bool{}, nil
		case reflect.String:
			return []string{}, nil
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return []int64{}, nil
		case reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return []int64{}, nil
		case reflect.Float32, reflect.Float64:
			return []metadataFloat64{}, nil
		default:
			return nil, errors.Errorf("%s cannot use empty arrays of type %s", path, rv.Type().String())
		}
	}

	type scalarKind int
	const (
		scalarUnknown scalarKind = iota
		scalarBool
		scalarString
		scalarInt
		scalarFloat
	)

	kind := scalarUnknown
	var bools []bool
	var stringsOut []string
	var ints []int64
	var floats []metadataFloat64

	for i := 0; i < rv.Len(); i++ {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		normalized, err := normalizeMetadataValue(itemPath, rv.Index(i).Interface(), false)
		if err != nil {
			return nil, err
		}

		switch v := normalized.(type) {
		case bool:
			if kind != scalarUnknown && kind != scalarBool {
				return nil, errors.Errorf("%s must be a homogeneous array of bool, int, float, or string", path)
			}
			kind = scalarBool
			if bools == nil {
				bools = make([]bool, 0, rv.Len())
			}
			bools = append(bools, v)
		case string:
			if kind != scalarUnknown && kind != scalarString {
				return nil, errors.Errorf("%s must be a homogeneous array of bool, int, float, or string", path)
			}
			kind = scalarString
			if stringsOut == nil {
				stringsOut = make([]string, 0, rv.Len())
			}
			stringsOut = append(stringsOut, v)
		case int64:
			if kind == scalarUnknown || kind == scalarInt {
				kind = scalarInt
				if ints == nil {
					ints = make([]int64, 0, rv.Len())
				}
				ints = append(ints, v)
				continue
			}
			if kind == scalarFloat {
				if floats == nil {
					floats = make([]metadataFloat64, 0, rv.Len())
				}
				floats = append(floats, metadataFloat64(float64(v)))
				continue
			}
			return nil, errors.Errorf("%s must be a homogeneous array of bool, int, float, or string", path)
		case metadataFloat64:
			if kind == scalarUnknown {
				kind = scalarFloat
			}
			if kind == scalarInt {
				if floats == nil {
					floats = make([]metadataFloat64, 0, rv.Len())
				}
				for _, iv := range ints {
					floats = append(floats, metadataFloat64(float64(iv)))
				}
				ints = nil
				kind = scalarFloat
			}
			if kind != scalarFloat {
				return nil, errors.Errorf("%s must be a homogeneous array of bool, int, float, or string", path)
			}
			if floats == nil {
				floats = make([]metadataFloat64, 0, rv.Len())
			}
			floats = append(floats, v)
		default:
			return nil, errors.Errorf("%s has unsupported metadata array element type %T", itemPath, normalized)
		}
	}

	switch kind {
	case scalarBool:
		return bools, nil
	case scalarString:
		return stringsOut, nil
	case scalarInt:
		return ints, nil
	case scalarFloat:
		return floats, nil
	default:
		return nil, errors.Errorf("%s has unsupported metadata array type %s", path, rv.Type().String())
	}
}

func marshalRequestJSON(v any) ([]byte, error) {
	payload, err := json.Marshal(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal request")
	}
	return append(payload, 0), nil
}
