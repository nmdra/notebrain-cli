package chroma

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/pkg/errors"
)

var (
	libHandle uintptr
	libOnce   sync.Once
	libErr    error
	ffiMu     sync.Mutex

	// FFI functions
	chromaServerStart                       func(*byte) uintptr
	chromaServerStartFromString             func(*byte) uintptr
	chromaServerPort                        func(uintptr) int32
	chromaServerAddress                     func(uintptr) *byte
	chromaServerPersistPath                 func(uintptr) *byte
	chromaServerStop                        func(uintptr) int32
	chromaServerFree                        func(uintptr)
	chromaEmbeddedStart                     func(*byte) uintptr
	chromaEmbeddedStartFromString           func(*byte) uintptr
	chromaEmbeddedPersistPath               func(uintptr) *byte
	chromaEmbeddedFree                      func(uintptr)
	chromaEmbeddedHeartbeat                 func(uintptr, *uint64) int32
	chromaEmbeddedGetMaxBatchSize           func(uintptr, *uint32) int32
	chromaEmbeddedCreateTenant              func(uintptr, *byte) int32
	chromaEmbeddedGetTenant                 func(uintptr, *byte) *byte
	chromaEmbeddedUpdateTenant              func(uintptr, *byte) int32
	chromaEmbeddedReset                     func(uintptr) int32
	chromaEmbeddedCreateDatabase            func(uintptr, *byte) int32
	chromaEmbeddedListDatabases             func(uintptr, *byte) *byte
	chromaEmbeddedGetDatabase               func(uintptr, *byte) *byte
	chromaEmbeddedDeleteDatabase            func(uintptr, *byte) int32
	chromaEmbeddedListCollections           func(uintptr, *byte) *byte
	chromaEmbeddedGetCollection             func(uintptr, *byte) *byte
	chromaEmbeddedCountCollections          func(uintptr, *byte, *uint32) int32
	chromaEmbeddedUpdateCollection          func(uintptr, *byte) int32
	chromaEmbeddedDeleteCollection          func(uintptr, *byte) int32
	chromaEmbeddedForkCollection            func(uintptr, *byte) *byte
	chromaEmbeddedCount                     func(uintptr, *byte, *uint32) int32
	chromaEmbeddedGet                       func(uintptr, *byte) *byte
	chromaEmbeddedUpdate                    func(uintptr, *byte) int32
	chromaEmbeddedUpsert                    func(uintptr, *byte) int32
	chromaEmbeddedDeleteRecordsWithResponse func(uintptr, *byte) *byte
	chromaEmbeddedCreateCollection          func(uintptr, *byte) *byte
	chromaEmbeddedAdd                       func(uintptr, *byte) int32
	chromaEmbeddedQuery                     func(uintptr, *byte) *byte
	chromaEmbeddedIndexingStatus            func(uintptr, *byte) *byte
	chromaEmbeddedHealthcheck               func(uintptr) *byte
	chromaEmbeddedRebuildCollection         func(uintptr, *byte) *byte
	chromaEmbeddedCompactCollection         func(uintptr, *byte) *byte
	chromaEmbeddedCompactAll                func(uintptr, *byte) *byte
	chromaEmbeddedPruneWALCollection        func(uintptr, *byte) *byte
	chromaEmbeddedPruneWALAll               func(uintptr, *byte) *byte
	chromaStringFree                        func(*byte)
	chromaGetLastError                      func() *byte
	chromaVersion                           func() *byte
)

const maxCStringLen = 1 << 20

// Init initializes the Chroma library. Must be called before any other functions.
// If libPath is empty, it will look for CHROMA_LIB_PATH environment variable.
func Init(libPath string) error {
	libOnce.Do(func() {
		libHandle, libErr = loadLibrary(libPath)
		if libErr != nil {
			return
		}
		libErr = registerFunctions()
	})
	return libErr
}

func registerFunctions() error {
	registrations := []struct {
		target any
		name   string
	}{
		{&chromaServerStart, "chroma_server_start"},
		{&chromaServerStartFromString, "chroma_server_start_from_string"},
		{&chromaServerPort, "chroma_server_port"},
		{&chromaServerAddress, "chroma_server_address"},
		{&chromaServerPersistPath, "chroma_server_persist_path"},
		{&chromaServerStop, "chroma_server_stop"},
		{&chromaServerFree, "chroma_server_free"},
		{&chromaEmbeddedStart, "chroma_embedded_start"},
		{&chromaEmbeddedStartFromString, "chroma_embedded_start_from_string"},
		{&chromaEmbeddedPersistPath, "chroma_embedded_persist_path"},
		{&chromaEmbeddedFree, "chroma_embedded_free"},
		{&chromaEmbeddedHeartbeat, "chroma_embedded_heartbeat"},
		{&chromaEmbeddedGetMaxBatchSize, "chroma_embedded_get_max_batch_size"},
		{&chromaEmbeddedCreateTenant, "chroma_embedded_create_tenant"},
		{&chromaEmbeddedGetTenant, "chroma_embedded_get_tenant"},
		{&chromaEmbeddedUpdateTenant, "chroma_embedded_update_tenant"},
		{&chromaEmbeddedReset, "chroma_embedded_reset"},
		{&chromaEmbeddedCreateDatabase, "chroma_embedded_create_database"},
		{&chromaEmbeddedListDatabases, "chroma_embedded_list_databases"},
		{&chromaEmbeddedGetDatabase, "chroma_embedded_get_database"},
		{&chromaEmbeddedDeleteDatabase, "chroma_embedded_delete_database"},
		{&chromaEmbeddedListCollections, "chroma_embedded_list_collections"},
		{&chromaEmbeddedGetCollection, "chroma_embedded_get_collection"},
		{&chromaEmbeddedCountCollections, "chroma_embedded_count_collections"},
		{&chromaEmbeddedUpdateCollection, "chroma_embedded_update_collection"},
		{&chromaEmbeddedDeleteCollection, "chroma_embedded_delete_collection"},
		{&chromaEmbeddedForkCollection, "chroma_embedded_fork_collection"},
		{&chromaEmbeddedCount, "chroma_embedded_count"},
		{&chromaEmbeddedGet, "chroma_embedded_get"},
		{&chromaEmbeddedUpdate, "chroma_embedded_update"},
		{&chromaEmbeddedUpsert, "chroma_embedded_upsert"},
		{&chromaEmbeddedDeleteRecordsWithResponse, "chroma_embedded_delete_records_with_response"},
		{&chromaEmbeddedCreateCollection, "chroma_embedded_create_collection"},
		{&chromaEmbeddedAdd, "chroma_embedded_add"},
		{&chromaEmbeddedQuery, "chroma_embedded_query"},
		{&chromaEmbeddedIndexingStatus, "chroma_embedded_indexing_status"},
		{&chromaEmbeddedHealthcheck, "chroma_embedded_healthcheck"},
		{&chromaEmbeddedRebuildCollection, "chroma_embedded_rebuild_collection"},
		{&chromaEmbeddedCompactCollection, "chroma_embedded_compact_collection"},
		{&chromaEmbeddedCompactAll, "chroma_embedded_compact_all"},
		{&chromaEmbeddedPruneWALCollection, "chroma_embedded_prune_wal_collection"},
		{&chromaEmbeddedPruneWALAll, "chroma_embedded_prune_wal_all"},
		{&chromaStringFree, "chroma_string_free"},
		{&chromaGetLastError, "chroma_get_last_error"},
		{&chromaVersion, "chroma_version"},
	}

	for _, registration := range registrations {
		if err := registerLibFunction(registration.target, registration.name); err != nil {
			return err
		}
	}

	return nil
}

func registerLibFunction(target any, name string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.Errorf("failed to register FFI symbol %q: %v", name, recovered)
		}
	}()
	purego.RegisterLibFunc(target, libHandle, name)
	return nil
}

func nullPointerError(details string) error {
	details = strings.TrimSpace(details)
	if details == "" {
		return ErrNullPointer
	}
	return fmt.Errorf("%w: %s", ErrNullPointer, details)
}

func callFFIHandle(call func() uintptr) (uintptr, error) {
	ffiMu.Lock()
	defer ffiMu.Unlock()

	handle := call()
	if handle == 0 {
		return 0, nullPointerError(getLastErrorUnlocked())
	}
	return handle, nil
}

func callFFIPointer(call func() *byte) (*byte, error) {
	ffiMu.Lock()
	defer ffiMu.Unlock()

	ptr := call()
	if ptr == nil {
		return nil, nullPointerError(getLastErrorUnlocked())
	}
	return ptr, nil
}

func getLastErrorUnlocked() string {
	ptr := chromaGetLastError()
	if ptr == nil {
		return ""
	}
	defer chromaStringFree(ptr)
	return goStringFromPtr(ptr)
}

func goStringFromPtr(ptr *byte) string {
	if ptr == nil {
		return ""
	}
	var n uintptr
	q := unsafe.Pointer(ptr)
	for n < maxCStringLen && *(*byte)(unsafe.Add(q, n)) != 0 {
		n++
	}
	return string(unsafe.Slice(ptr, n))
}

func cStringFromGo(s string) []byte {
	return append([]byte(s), 0)
}

// Server represents a running Chroma server instance.
type Server struct {
	stateMu  sync.RWMutex
	backupMu sync.Mutex

	handle      uintptr
	port        int
	addr        string
	config      StartServerConfig
	persistPath string
}

// StartServerConfig contains configuration options for starting a server.
type StartServerConfig struct {
	ConfigPath   string // Path to YAML config file
	ConfigString string // YAML config string (used if ConfigPath is empty)
}

// StartServer starts a new Chroma server with the given configuration.
func StartServer(config StartServerConfig) (*Server, error) {
	if libHandle == 0 {
		return nil, ErrLibraryNotLoaded
	}

	var handle uintptr
	var err error
	switch {
	case config.ConfigPath != "":
		pathBytes := cStringFromGo(config.ConfigPath)
		handle, err = callFFIHandle(func() uintptr { return chromaServerStart(&pathBytes[0]) })
	case config.ConfigString != "":
		yamlBytes := cStringFromGo(config.ConfigString)
		handle, err = callFFIHandle(func() uintptr { return chromaServerStartFromString(&yamlBytes[0]) })
	default:
		return nil, errors.New("either ConfigPath or ConfigString must be provided")
	}
	if err != nil {
		return nil, err
	}

	var port int32
	addr := ""
	persistPath := ""
	func() {
		ffiMu.Lock()
		defer ffiMu.Unlock()
		port = chromaServerPort(handle)
		addrPtr := chromaServerAddress(handle)
		if addrPtr != nil {
			addr = goStringFromPtr(addrPtr)
		}
		persistPathPtr := chromaServerPersistPath(handle)
		if persistPathPtr != nil {
			persistPath = goStringFromPtr(persistPathPtr)
		}
	}()

	resolvedPersistPath, persistPathErr := normalizePersistPath(persistPath)
	if persistPathErr != nil {
		baseErr := errors.Wrap(persistPathErr, "failed to resolve persist path from runtime config")

		ffiMu.Lock()
		stopRC := chromaServerStop(handle)
		stopErrMsg := ""
		if stopRC != Success {
			stopErrMsg = getLastErrorUnlocked()
		}
		chromaServerFree(handle)
		ffiMu.Unlock()

		if stopRC != Success {
			stopErr := errorFromCode(stopRC, stopErrMsg)
			return nil, fmt.Errorf("%w; cleanup stop failed: %v", baseErr, stopErr)
		}
		return nil, baseErr
	}

	server := &Server{
		handle:      handle,
		port:        int(port),
		addr:        addr,
		config:      config,
		persistPath: resolvedPersistPath,
	}

	runtime.SetFinalizer(server, func(s *Server) {
		_ = s.Close()
	})

	return server, nil
}

// Port returns the port the server is listening on.
func (s *Server) Port() int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.port
}

// Address returns the address the server is listening on.
func (s *Server) Address() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.addr
}

// URL returns the full URL of the server (e.g., "http://127.0.0.1:8000").
func (s *Server) URL() string {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return fmt.Sprintf("http://%s:%d", s.addr, s.port)
}

// Stop gracefully stops the server.
func (s *Server) Stop() error {
	if s == nil {
		return ErrServerNotStarted
	}
	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(s)

	handle := atomic.LoadUintptr(&s.handle)
	if handle == 0 {
		return ErrServerNotStarted
	}

	rc := chromaServerStop(handle)
	if rc != Success {
		return errorFromCode(rc, getLastErrorUnlocked())
	}
	return nil
}

// Close stops the server and frees resources.
func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	ffiMu.Lock()
	defer ffiMu.Unlock()
	defer runtime.KeepAlive(s)

	handle := atomic.SwapUintptr(&s.handle, 0)
	if handle == 0 {
		return nil
	}

	stopRC := chromaServerStop(handle)
	stopErrMsg := ""
	if stopRC != Success {
		stopErrMsg = getLastErrorUnlocked()
	}
	chromaServerFree(handle)

	if stopRC != Success {
		stopErr := errorFromCode(stopRC, stopErrMsg)
		if errors.Is(stopErr, ErrServerAlreadyStop) {
			return nil
		}
		return stopErr
	}
	return nil
}

// Version returns the version of the Chroma shim library.
func Version() string {
	version, _ := VersionWithError()
	return version
}

// VersionWithError returns the version of the Chroma shim library.
func VersionWithError() (string, error) {
	if libHandle == 0 {
		return "", ErrLibraryNotLoaded
	}
	// chroma_version returns a static C string owned by Rust; do not free it.
	ptr, err := callFFIPointer(func() *byte { return chromaVersion() })
	if err != nil {
		return "", err
	}
	return goStringFromPtr(ptr), nil
}
