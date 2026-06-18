# local-go-chroma

A minimal Go wrapper for running Chroma from Go using a Rust FFI shim and [purego](https://github.com/ebitengine/purego) (no cgo required).

It supports both:
- server mode (starts the HTTP frontend)
- embedded mode (direct in-process calls, no HTTP port)

## Requirements

- Go 1.21+
- Rust 1.70+
- Java 17+ (JNA module) and Java 22+ (Panama module)
- `golangci-lint` (for `lint` checks)

## Supported Platform Matrix

The table below captures current support and CI coverage for this repository.

| OS | Arch | CI coverage | Release shim archive | Notes |
|---|---|---|---|---|
| Linux | amd64 | yes | yes | Fully exercised in CI. |
| macOS | amd64 | no | no | Not in current hosted CI matrix. |
| Windows | amd64 | yes | yes | Use the documented PowerShell workflow for local dev. |
| Linux | arm64 | no | no | Not in current hosted CI matrix. |
| macOS | arm64 | yes | yes | Fully exercised in CI. |
| Windows | arm64 | no | no | Toolchain is documented, but CI/release artifacts are not yet published. |

See [Prebuilt Release Artifacts](#prebuilt-release-artifacts) for release asset naming and [Windows toolchain setup](#windows-toolchain-setup) for local Windows prerequisites.

## Integration Direction (`chroma-go` PersistentClient)

This repository is the low-level runtime layer (`purego` + Rust shim). It is intended to be consumed by `github.com/amikos-tech/chroma-go` for a downstream `PersistentClient`.

Design intent:

- `local-go-chroma` remains independent and does not import `chroma-go`
- `chroma-go` depends on `local-go-chroma` to embed Chroma in Go apps
- integration and compatibility tests for `PersistentClient` should live in `chroma-go`

## Building

```bash
# Build debug version
make build

# Build release version
make build-release
```

## Java Scaffold

Java bindings are scaffolded under `java/`:

- `java/core` (Java 17): shared runtime interface and error/session types
- `java/jna` (Java 17): JNA backend
- `java/panama` (Java 22): Foreign Function & Memory API backend

```bash
# Build Java modules
make build-java

# Run Java smoke tests (expects CHROMA_LIB_PATH, auto-set by make target)
make test-java
```

Local Java builds default to artifact version `0.0.0-SNAPSHOT`. Tag releases pass the repository tag version into Gradle so Java JARs track the same release line as the native shim.

## Windows Developer Workflow (PowerShell)

Use the PowerShell helper on Windows for native build/test/lint parity:

```powershell
pwsh -File .\scripts\dev-windows.ps1 -Task help
```

On Windows, prefer the PowerShell workflow for `test`, `test-release`, and `bench-go`; these Make targets are intentionally guarded on Windows Make hosts to avoid path translation issues.

### Windows toolchain setup

1. Install Go 1.21+.
2. Install Rust with an MSVC target toolchain:

```powershell
# x64 Windows
rustup toolchain install stable-x86_64-pc-windows-msvc
rustup default stable-x86_64-pc-windows-msvc
```

```powershell
# ARM64 Windows
rustup toolchain install stable-aarch64-pc-windows-msvc
rustup default stable-aarch64-pc-windows-msvc
```

3. Install `protoc` 31.x (matches Chroma `1.5.5` toolchain and this repo's CI).
4. Install `golangci-lint`.
5. Install `goimports`:

```powershell
go install golang.org/x/tools/cmd/goimports@latest
```

### Common Windows commands

```powershell
# Build debug shim
pwsh -File .\scripts\dev-windows.ps1 -Task build

# Run Go tests (builds debug shim and sets CHROMA_LIB_PATH automatically)
pwsh -File .\scripts\dev-windows.ps1 -Task test

# Run Rust tests
pwsh -File .\scripts\dev-windows.ps1 -Task test-rust

# Run linters (golangci-lint + cargo clippy)
pwsh -File .\scripts\dev-windows.ps1 -Task lint
```

## Prebuilt Release Artifacts

Tag pushes matching `v*` trigger `.github/workflows/release.yml`, which performs:

- GitHub release upload (for compatibility)
- signed native shim archives and Java JARs uploaded to `https://releases.amikos.tech/chroma-go-local/<version>/`
- `latest.json` update at `https://releases.amikos.tech/chroma-go-local/latest.json`
- signed `releases.json` index update at `https://releases.amikos.tech/chroma-go-local/releases.json`

Canonical release asset naming:

- `chroma-go-local-<version>-linux-<arch>.tar.gz`
- `chroma-go-local-<version>-darwin-<arch>.tar.gz`
- `chroma-go-local-<version>-windows-<arch>.tar.gz`
- `chroma-local-java-core-<version>.jar`
- `chroma-local-java-jna-<version>.jar`
- `chroma-local-java-panama-<version>.jar`
- `SHA256SUMS`
- `*.sigstore.json` for each release asset and `SHA256SUMS`
- `*.sig` + `*.pem` for each release asset and `SHA256SUMS` (for users verifying with Cosign v2)

Architecture note: native archive `<arch>` is derived from the GitHub runner architecture. In the current hosted matrix for this repository, Linux/Windows builds are `amd64` and macOS builds are `arm64`. Runner mappings can change over time.

Library filename mapping inside each archive:

| OS | Library filename |
|---|---|
| Linux | `libchroma_shim.so` |
| macOS | `libchroma_shim.dylib` |
| Windows | `chroma_shim.dll` |

Native shim archive example usage:

```bash
# Linux/macOS
tar -xzf chroma-go-local-v<version>-linux-amd64.tar.gz
export CHROMA_LIB_PATH="$(pwd)/libchroma_shim.so"
```

```powershell
# Windows PowerShell
tar -xzf chroma-go-local-v<version>-windows-amd64.tar.gz
$env:CHROMA_LIB_PATH = (Resolve-Path .\chroma_shim.dll).Path
```

Verify release checksums:

```bash
# Linux
sha256sum -c SHA256SUMS

# macOS
shasum -a 256 -c SHA256SUMS
```

```powershell
# Windows PowerShell
Get-Content SHA256SUMS | ForEach-Object {
    if (-not $_) { return }
    $expected, $file = $_ -split '  ', 2
    $actual = (Get-FileHash -Algorithm SHA256 $file).Hash.ToLowerInvariant()
    if ($actual -eq $expected) { "OK: $file" } else { throw "MISMATCH: $file" }
}
```

Verify signatures (cosign keyless):

```bash
# Requires Cosign v3.0.0 or later.
cosign verify-blob \
  --bundle SHA256SUMS.sigstore.json \
  --certificate-identity "https://github.com/amikos-tech/chroma-go-local/.github/workflows/release.yml@refs/tags/v<version>" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  --use-signed-timestamps \
  SHA256SUMS
```

Cosign v3 bundles (`*.sigstore.json`) are the primary verification material and the only inputs used by the release workflow's own verification step. Detached `*.sig` and `*.pem` files are also published for users verifying with Cosign v2.

Breaking change in `v0.3.1`: shared library filenames changed from `chroma_go_shim` to `chroma_shim`.

Backfill older tags to R2 using the latest workflow definition (`main`) while targeting historical tags:

```bash
./scripts/backfill-r2.sh --workflow-ref main v0.1.0 v0.2.0 v0.3.0
```

`latest.json` includes `checksums_url` (project-relative path) and `checksums_full_url` (absolute URL).

## Usage

```go
package main

import (
    "fmt"
    "os"

    chroma "github.com/amikos-tech/chroma-go-local"
)

func main() {
    // Initialize - set CHROMA_LIB_PATH or pass path directly
    if err := chroma.Init(""); err != nil {
        fmt.Fprintf(os.Stderr, "Failed to initialize: %v\n", err)
        os.Exit(1)
    }

    // Start server with builder pattern
    server, err := chroma.NewServer(
        chroma.WithPort(8000),
        chroma.WithPersistPath("./chroma_data"),
        chroma.WithAllowReset(true),
    )
    if err != nil {
        fmt.Fprintf(os.Stderr, "Failed to start server: %v\n", err)
        os.Exit(1)
    }
    defer server.Close()

    fmt.Printf("Server running at %s\n", server.URL())

    // Use any Chroma client to connect to the server...
}
```

## Embedded Mode (No HTTP Port)

```go
embedded, err := chroma.NewEmbedded(
    chroma.WithEmbeddedPersistPath("./chroma_data"),
    chroma.WithEmbeddedAllowReset(true),
)
if err != nil {
    panic(err)
}
defer embedded.Close()

collection, err := embedded.CreateCollection(chroma.EmbeddedCreateCollectionRequest{
    Name: "docs",
    Metadata: map[string]any{
        "owner": "qa",
        "active": true,
    },
    Configuration: map[string]any{
        "hnsw": map[string]any{
            "space": "cosine",
        },
    },
    GetOrCreate: true,
})
if err != nil {
    panic(err)
}

// Response includes metadata, configuration_json, and schema.
fmt.Println(collection.Metadata["owner"])
fmt.Println(collection.ConfigurationJSON["hnsw"])

// You can also create a collection from an existing schema.
schemaCopy, err := embedded.CreateCollection(chroma.EmbeddedCreateCollectionRequest{
    Name:        "docs_schema_copy",
    Schema:      collection.Schema,
    GetOrCreate: true,
})
if err != nil {
    panic(err)
}
fmt.Println(schemaCopy.ID)

err = embedded.Add(chroma.EmbeddedAddRequest{
    CollectionID: collection.ID,
    IDs:          []string{"doc-1"},
    Embeddings:   [][]float32{{0.1, 0.2, 0.3}},
})
if err != nil {
    panic(err)
}

result, err := embedded.Query(chroma.EmbeddedQueryRequest{
    CollectionID:    collection.ID,
    QueryEmbeddings: [][]float32{{0.1, 0.2, 0.3}},
    NResults:        1,
})
if err != nil {
    panic(err)
}
fmt.Println(result.IDs)
```

`EmbeddedCreateCollectionRequest` fields:
- `Name`
- `TenantID` (optional)
- `DatabaseName` (optional)
- `Metadata` (optional)
- `Configuration` (optional)
- `Schema` (optional)
- `GetOrCreate` (optional)

`EmbeddedCollection` response fields include:
- `ID`, `Name`, `Tenant`, `Database`
- `Metadata`
- `ConfigurationJSON` (JSON key: `configuration_json`)
- `Schema`

`EmbeddedUpdateCollectionRequest` fields:
- `CollectionID`
- `NewName` (optional)
- `NewMetadata` (optional; replaces existing collection metadata; nil values are rejected)
- `DatabaseName` (optional)

At least one of `NewName` or `NewMetadata` is required.

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `WithPort(port)` | Server port | 8000 |
| `WithListenAddress(addr)` | Bind address | "127.0.0.1" |
| `WithPersistPath(path)` | Data directory | "./chroma" |
| `WithAllowReset(bool)` | Enable reset endpoint | false |
| `WithMaxPayloadSize(bytes)` | Max request size | 40 MB |
| `WithCORSAllowOrigins(origins...)` | CORS allowed origins | none |
| `WithSQLiteFilename(name)` | SQLite DB filename | "chroma.sqlite3" |
| `WithOpenTelemetry(endpoint, service)` | Enable OTel tracing | disabled |
| `WithRawYAML(yaml)` | Raw YAML config (overrides all) | - |

### Alternative: YAML config file

```go
server, err := chroma.StartServer(chroma.StartServerConfig{
    ConfigPath: "./config.yaml",
})
```

### Alternative: Inline YAML string

```go
server, err := chroma.StartServer(chroma.StartServerConfig{
    ConfigString: `
port: 8000
persist_path: "./chroma_data"
allow_reset: true
`,
})
```

## API

For a detailed, example-heavy reference of the currently implemented Go APIs, see [`GO_API_SURFACE.md`](GO_API_SURFACE.md).
For the Java scaffold surface, see [`JAVA_API_SURFACE.md`](JAVA_API_SURFACE.md).

| Function | Description |
|----------|-------------|
| `Init(libPath string) error` | Initialize the library. Uses `CHROMA_LIB_PATH` env if path is empty. |
| `Version() string` | Returns the shim version. |
| `VersionWithError() (string, error)` | Returns shim version with explicit error details. |
| `NewServer(opts ...ServerOption) (*Server, error)` | Start a server with builder options. |
| `StartServer(config StartServerConfig) (*Server, error)` | Start a server with YAML config. |
| `(*Server) Port() int` | Get the server port. |
| `(*Server) Address() string` | Get the server listen address. |
| `(*Server) URL() string` | Get the full server URL. |
| `(*Server) Stop() error` | Gracefully stop the server. |
| `(*Server) Close() error` | Stop and free resources. |
| `(*Server) Backup(options ...BackupOption) (*BackupManifest, error)` | Snapshot persisted data with optional restart. |
| `(*Server) RebuildCollection(name string, options ...RebuildCollectionOption) (*RebuildCollectionResult, error)` | Rebuild persisted vector index artifacts for one collection (server restarts after operation). |
| `(*Server) CompactCollection(request CompactCollectionRequest) (*CompactionResult, error)` | Run explicit compaction for one collection (server restarts after operation). Scope can include both `TenantID` and `DatabaseName` together. |
| `(*Server) CompactAll(request CompactAllRequest) (*CompactionResult, error)` | Run explicit compaction for all collections (server restarts after operation). Scope can include both `TenantID` and `DatabaseName` together. Per-collection failures are reported in `result.Collections[i].Error`. |
| `(*Server) PruneCollectionWAL(name string, options ...WALPruneOption) (*WALPruneResult, error)` | Run explicit WAL prune for one collection (server restarts after operation). Scope can include `WithWALPruneTenantID(...)` + `WithWALPruneDatabaseName(...)`. |
| `(*Server) PruneAllWAL(options ...WALPruneOption) (*WALPruneResult, error)` | Run explicit WAL prune for all collections in scope (server restarts after operation). Per-collection failures are reported in `result.Collections[i].Error`. |
| `NewEmbedded(opts ...EmbeddedOption) (*Embedded, error)` | Start in-process embedded mode. |
| `StartEmbedded(config StartEmbeddedConfig) (*Embedded, error)` | Start embedded mode from YAML config. |
| `(*Embedded) Heartbeat() (uint64, error)` | Read in-process heartbeat nanoseconds. |
| `(*Embedded) MaxBatchSize() (uint32, error)` | Get embedded max batch size. |
| `(*Embedded) CreateTenant(request EmbeddedCreateTenantRequest) error` | Create a tenant in embedded mode. |
| `(*Embedded) GetTenant(request EmbeddedGetTenantRequest) (*EmbeddedTenant, error)` | Get a tenant by name. |
| `(*Embedded) UpdateTenant(request EmbeddedUpdateTenantRequest) error` | Update tenant properties. |
| `(*Embedded) Healthcheck() (*EmbeddedHealthCheckResponse, error)` | Get embedded readiness state. |
| `(*Embedded) CreateDatabase(request EmbeddedCreateDatabaseRequest) error` | Create a database in embedded mode. |
| `(*Embedded) ListDatabases(request EmbeddedListDatabasesRequest) ([]EmbeddedDatabase, error)` | List databases in embedded mode. |
| `(*Embedded) GetDatabase(request EmbeddedGetDatabaseRequest) (*EmbeddedDatabase, error)` | Get a database by name. |
| `(*Embedded) DeleteDatabase(request EmbeddedDeleteDatabaseRequest) error` | Delete a database by name. |
| `(*Embedded) CreateCollection(request EmbeddedCreateCollectionRequest) (*EmbeddedCollection, error)` | Create a collection without HTTP, including optional metadata/configuration/schema. |
| `(*Embedded) ListCollections(request EmbeddedListCollectionsRequest) ([]EmbeddedCollection, error)` | List collections for a database (includes metadata/configuration_json/schema in each item). |
| `(*Embedded) GetCollection(request EmbeddedGetCollectionRequest) (*EmbeddedCollection, error)` | Get a collection by name (includes metadata/configuration_json/schema). |
| `(*Embedded) CountCollections(request EmbeddedCountCollectionsRequest) (uint32, error)` | Count collections for a database. |
| `(*Embedded) UpdateCollection(request EmbeddedUpdateCollectionRequest) error` | Update a collection name and/or metadata. |
| `(*Embedded) DeleteCollection(request EmbeddedDeleteCollectionRequest) error` | Delete a collection by name. |
| `(*Embedded) ForkCollection(request EmbeddedForkCollectionRequest) (*EmbeddedCollection, error)` | Fork a collection (may be unimplemented in local mode). |
| `(*Embedded) CountRecords(request EmbeddedCountRecordsRequest) (uint32, error)` | Count records for a collection. |
| `(*Embedded) GetRecords(request EmbeddedGetRecordsRequest) (*EmbeddedGetRecordsResponse, error)` | Get records from a collection (supports `where` and `where_document`). |
| `(*Embedded) UpdateRecords(request EmbeddedUpdateRecordsRequest) error` | Update existing records by id. |
| `(*Embedded) UpsertRecords(request EmbeddedUpsertRecordsRequest) error` | Upsert records by id. |
| `(*Embedded) DeleteRecords(request EmbeddedDeleteRecordsRequest) error` | Delete records by ids and/or filters. Discards the deleted-count response for backward compatibility. |
| `(*Embedded) DeleteRecordsWithResponse(request EmbeddedDeleteRecordsRequest) (*EmbeddedDeleteRecordsResponse, error)` | Delete records by ids and/or filters and return `{deleted}`. Supports optional `limit` with `where` / `where_document` (`limit` must be greater than zero). |
| `(*Embedded) Add(request EmbeddedAddRequest) error` | Add records without HTTP. |
| `(*Embedded) Query(request EmbeddedQueryRequest) (*EmbeddedQueryResponse, error)` | Query records without HTTP (supports `where` and `where_document`). |
| `(*Embedded) IndexingStatus(request EmbeddedIndexingStatusRequest) (*EmbeddedIndexingStatusResponse, error)` | Get collection indexing status (may be unimplemented in local backend). |
| `(*Embedded) RebuildCollection(name string, options ...RebuildCollectionOption) (*RebuildCollectionResult, error)` | Rebuild persisted vector index artifacts for one collection. |
| `(*Embedded) CompactCollection(request CompactCollectionRequest) (*CompactionResult, error)` | Run explicit compaction for one collection. Scope can include both `TenantID` and `DatabaseName` together. |
| `(*Embedded) CompactAll(request CompactAllRequest) (*CompactionResult, error)` | Run explicit compaction for all collections. Scope can include both `TenantID` and `DatabaseName` together. Per-collection failures are reported in `result.Collections[i].Error`. |
| `(*Embedded) PruneCollectionWAL(name string, options ...WALPruneOption) (*WALPruneResult, error)` | Run explicit WAL prune for one collection. Scope can include `WithWALPruneTenantID(...)` + `WithWALPruneDatabaseName(...)`. |
| `(*Embedded) PruneAllWAL(options ...WALPruneOption) (*WALPruneResult, error)` | Run explicit WAL prune for all collections in scope. Per-collection failures are reported in `result.Collections[i].Error`. |
| `(*Embedded) Reset() error` | Reset local state when enabled. |
| `(*Embedded) Backup(options ...BackupOption) (*BackupManifest, error)` | Snapshot persisted data with optional reopen. |
| `(*Embedded) Close() error` | Free embedded resources. |

### Compaction Semantics

`CompactCollection` and `CompactAll` run Chroma explicit compaction through the local compaction manager.

Per compacted collection, the operation performs:
- backfill (apply pending log operations into collection/index state)
- log purge (remove compacted WAL log records)

This compaction is not a full index rebuild. In particular, it does not rebuild HNSW from scratch or change collection configuration/schema.

Operational notes:
- You can scope compaction with both `TenantID` and `DatabaseName` in the same request.
- For `CompactCollection`, collection name resolution happens inside that tenant+database scope.
- If omitted, scope defaults to `default_tenant` and `default_database`.
- In server mode, the server is unavailable during compaction because it is stopped, compacted via embedded mode, then restarted.
- `CompactAll` continues across collections and records per-collection failures in `result.Collections[i].Error`.
- `result.CollectionCount` is the number of attempted collections (including entries with `Error`).
- `pending_ops_before`/`pending_ops_after` are advisory metrics; when unavailable they are omitted and `pending_ops_before_error`/`pending_ops_after_error` explain why.

Example with explicit tenant+database scope:

```go
result, err := server.CompactCollection(chroma.CompactCollectionRequest{
    Name:         "docs",
    TenantID:     "team_a",
    DatabaseName: "prod_db",
})
if err != nil {
    panic(err)
}
fmt.Println(result.CollectionCount)
```

### WAL Prune Semantics

`PruneCollectionWAL` and `PruneAllWAL` prune only safety-eligible WAL rows (below the minimum segment max-seq boundary for each collection).

Operational notes:
- Scope is controlled with `WithWALPruneTenantID(...)` and `WithWALPruneDatabaseName(...)` (same defaults as other maintenance APIs when omitted).
- Dry-run mode (`WithWALPruneDryRun()`) reports candidates/projections without mutating rows.
- Mutating prune calls require at least one retention policy:
  - `WithWALPruneMaxAge(duration)`
  - `WithWALPruneMaxBytes(bytes)` (`0` means "prune all safety-eligible candidates" for this policy)
  - `WithWALPruneWatermark(highBytes, lowBytes)`
- Multiple policies combine with AND semantics (a row must satisfy all configured strategies).
- `PruneAllWAL` continues across collections and reports per-collection failures in `result.Collections[i].Error`.
- Optional `WithWALPruneVacuum()` runs SQLite `VACUUM` once after prune execution (skipped in dry-run mode).
- `result.Warning` is set when prune succeeds but a follow-up vacuum step fails.
- For max-age policy, rows with NULL/invalid `created_at` are treated as UNIX epoch (`0`) and therefore considered oldest.
- In dry-run mode, `pruned_*` fields are projected counts/bytes rather than applied mutations.
- The shim logic adapts core flow from Chroma's Rust CLI vacuum command (migration + purge + optional vacuum), but does not depend on the CLI crate.

Recommended ordering for heavy maintenance:
- run `CompactCollection`/`CompactAll` first when backfill is needed
- run WAL prune with explicit policy controls
- optionally run vacuum when disk-file shrink is desired
- use rebuild only for index repair scenarios

### Rebuild vs WAL Prune vs Compaction vs Vacuum

`RebuildCollection` is a low-level maintenance primitive for rebuilding one collection's persisted vector index artifacts.

- Use `RebuildCollection` when index artifacts are inconsistent/corrupted or you need a full index rewrite for a specific collection.
- Use `CompactCollection`/`CompactAll` for WAL backfill + purge maintenance; compaction is not a full HNSW rebuild.
- Use `PruneCollectionWAL`/`PruneAllWAL` for explicit policy-driven WAL retention pruning.
- Vacuum can be requested through WAL prune (`WithWALPruneVacuum()`) when physical file shrink is needed.

Rebuild options:

- `WithRebuildTenantID(string)` (optional scope; must be at least 3 chars when set)
- `WithRebuildDatabaseName(string)` (optional scope; must be at least 3 chars when set)
- `WithRebuildPrecheck()` (prerequisites-only, no mutation)
- `WithRebuildKeepBackup(bool)` (default `true`; keep timestamped backup path after swap)

In server mode, rebuild runs with stop -> temporary embedded rebuild -> restart lifecycle, so the server is unavailable during the operation.
If rebuild itself succeeds but temporary close/restart fails, the API returns both a non-nil `RebuildCollectionResult` and a non-nil error.

### Backup API

Backup writes a consistent snapshot for either managed server mode or embedded mode:

- destination directory: `<destination>/persist`
- manifest file: `<destination>/backup_manifest.json`

Backup options:

- `WithDestination(path string)` (required; must be provided exactly once)
- `WithIncludeMetadata()` (optional; include per-file metadata in manifest)
- `WithLeaveStopped()` (optional; server backups only)
- `WithLeaveClosed()` (optional; embedded backups only)

Backup validates all provided options before side effects (close/restart/reopen). Invalid or mode-incompatible options return an error.

When `WithIncludeMetadata()` is used, each manifest file entry includes:
- `path`
- `size_bytes`
- `mode` (octal string, for example `"0644"`)
- `sha256` (hex-encoded SHA-256 of copied file bytes)
- `modified_at`

Constraints and error conditions:

- `DestinationPath` must not exist or must be an empty directory.
- `<destination>/persist` must not be inside the source persist path.
  This containment check resolves symlinks.
- Symlinks inside the source persist tree are rejected and cause backup to fail.

Practical example (managed server mode):

```go
backupRoot := "./backups"
destination := filepath.Join(
    backupRoot,
    time.Now().UTC().Format("20060102-150405"),
)

manifest, err := srv.Backup(
    chroma.WithDestination(destination),
    chroma.WithIncludeMetadata(),
)
if err != nil {
    panic(err)
}

fmt.Printf("backup created: %s (%d files)\n", manifest.SnapshotPath, manifest.FileCount)

// Optional restore validation: start another server from the snapshot.
restored, err := chroma.NewServer(
    chroma.WithPort(8010),
    chroma.WithListenAddress("127.0.0.1"),
    chroma.WithPersistPath(manifest.SnapshotPath),
    chroma.WithAllowReset(true),
)
if err != nil {
    panic(err)
}
defer restored.Close()
```

### Metadata Value Rules (Embedded Record APIs)

For `EmbeddedAddRequest.Metadatas`, `EmbeddedUpdateRecordsRequest.Metadatas`, and `EmbeddedUpsertRecordsRequest.Metadatas`:

- Supported scalar values: `bool`, integer types, float types, `string`
- Supported arrays: homogeneous arrays of one supported scalar type
- Unsupported values: nested objects/maps, mixed-type arrays, structs

`UpdateRecords` and `UpsertRecords` allow `nil` metadata values to clear keys. Float metadata values are encoded with an explicit decimal representation to avoid integer/float array ambiguity at the Go/Rust boundary. On read-back through `EmbeddedGetRecordsResponse`, numeric metadata values decode as `float64` due standard Go JSON decoding.

## Testing

```bash
make test-go       # Run Go tests (unit + integration + property tests)
make test-rust     # Run Rust shim tests (unit + proptests + FFI integration)
make test-java     # Run Java smoke tests (JNA + Panama)
make test-all      # Run Go/Rust tests plus Java smoke tests (when Gradle is installed)
make test-release  # Run Go tests with release build
```

```powershell
# Windows PowerShell equivalents
pwsh -File .\scripts\dev-windows.ps1 -Task test
pwsh -File .\scripts\dev-windows.ps1 -Task test-rust
pwsh -File .\scripts\dev-windows.ps1 -Task test-all
pwsh -File .\scripts\dev-windows.ps1 -Task test-release
```

## CI

GitHub Actions runs a cross-platform matrix (`ubuntu-latest`, `macos-latest`, `windows-latest`) on pushes to `main` and pull requests. Each matrix job runs:

1. `cargo build --locked` in `shim/`
2. `go test -v ./...` with platform-specific `CHROMA_LIB_PATH`
3. `golangci-lint run ./...`
4. `cargo clippy --locked -- -D warnings` in `shim/`
5. Java JNA smoke tests on Java 17
6. Java Panama smoke tests on Java 22

Release tags (`v*`) run a separate workflow that builds canonical native archives plus Java release JARs, signs artifacts with cosign v3 keyless bundles, publishes to both GitHub Releases and `releases.amikos.tech`, and updates `latest.json` plus signed `releases.json`.

## Troubleshooting

### Dynamic loading (`Init` / `CHROMA_LIB_PATH`)

If `Init("")` fails, validate all of the following first:

- `CHROMA_LIB_PATH` should be absolute for clarity. Relative paths that include separators are also supported and resolved by the loader.
- The library filename matches your platform (see [Prebuilt Release Artifacts](#prebuilt-release-artifacts)).
- The library exists at that exact path.

Quick verification:

```bash
# Linux/macOS
echo "$CHROMA_LIB_PATH"
ls -l "$CHROMA_LIB_PATH"
```

```powershell
# Windows PowerShell
echo $env:CHROMA_LIB_PATH
Test-Path $env:CHROMA_LIB_PATH
```

### Linux and macOS

- If the loader reports file-not-found, confirm extension and filename are correct for the platform.
- If using release downloads, re-check archive checksums before loading.
- On macOS, if the downloaded file is quarantined by Gatekeeper, remove quarantine metadata:

```bash
xattr -dr com.apple.quarantine /path/to/libchroma_shim.dylib
```

### Windows

- Prefer the PowerShell helper commands in this README (`scripts/dev-windows.ps1`) instead of `make` for test/lint/bench flows.
- Ensure the Rust MSVC target is active and `protoc` 31.x is installed before running tests.
- If path issues appear, set `CHROMA_LIB_PATH` via `Resolve-Path` as shown in [Prebuilt Release Artifacts](#prebuilt-release-artifacts).

### Build and test failures

- `protoc` version mismatches are a common source of build failures; use `31.x`.
- If Rust or Go dependencies are corrupted locally, clear build outputs and rerun:

```bash
make clean
make test-go
```

## Benchmarks

```bash
make bench-go      # Run Go benchmarks
make bench-rust    # Run Rust criterion benchmarks
make bench         # Run both benchmark suites
```

## Project Structure

```
.
├── chroma.go       # Main Go wrapper
├── config.go       # Server config builder with WithXXX options
├── embedded.go     # Embedded (in-process) API
├── library.go      # Library loading via purego
├── errors.go       # Error codes and handling
├── chroma_test.go  # Tests
├── embedded_test.go # Embedded integration test
├── Makefile        # Build orchestration
├── java/           # Java scaffold modules (core, jna, panama)
├── JAVA_API_SURFACE.md # Java scaffold surface and status
├── scripts/
│   ├── dev-windows.ps1 # Windows build/test/lint helper
│   └── backfill-r2.sh  # Trigger R2 backfill for existing tags
├── examples/
│   ├── go/basic/   # Go example usage
│   └── java/basic/ # Java scaffold usage
└── shim/
    ├── Cargo.toml  # Rust dependencies
    └── src/
        └── lib.rs  # Rust FFI shim
```

## License

MIT
