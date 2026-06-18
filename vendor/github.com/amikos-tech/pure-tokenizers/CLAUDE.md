# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a CGo-free tokenizers library that provides Go bindings for Rust-based tokenizers using purego for FFI. The project creates platform-specific shared libraries (.so/.dylib/.dll) from Rust code and loads them dynamically in Go without requiring CGo.

### Key Features
- **HuggingFace Hub Integration**: Direct loading of tokenizers from any HuggingFace model
- **Automatic caching**: Both library binaries and HuggingFace tokenizers are cached locally
- **Cross-platform support**: Works on Windows, macOS, and Linux (including ARM)

## Release Process

### Separate Release Cycles
The project uses separate release cycles for Rust and Go:
- **Rust releases**: Tagged with `rust-vX.Y.Z` (e.g., `rust-v0.1.0`)
- **Go releases**: Tagged with `vX.Y.Z` (e.g., `v0.1.0`)

### Creating Releases

**Important**: For the first release or when no Rust releases exist, the Go release workflow will automatically build the Rust library locally as a fallback. However, it's recommended to create Rust releases first for better artifact management.

1. **Rust Library Release** (recommended to do first):
   ```bash
   git tag rust-v0.1.0
   git push origin rust-v0.1.0
   ```
   This triggers the `rust-release.yml` workflow which builds and releases library artifacts for all supported platforms.

2. **Go Module Release**:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```
   This triggers the `go-release.yml` workflow which creates a Go module release.
   - If Rust releases exist: Downloads and tests against the latest `rust-v*` release
   - If no Rust releases exist: Builds the Rust library locally for testing (fallback)

### CI Workflows
- **rust-ci.yml**: Runs on Rust code changes (src/**, Cargo.toml)
- **go-ci.yml**: Runs on Go code changes (**.go, go.mod)
- **ci.yml**: Main integration tests running on all changes
- **rust-release.yml**: Builds and releases Rust library artifacts
- **go-release.yml**: Creates Go module releases

### Troubleshooting Release Issues

**Issue: First release or no Rust artifacts available**
- The Go release workflow will automatically build the Rust library locally if no `rust-v*` releases are found
- This prevents the circular dependency issue where Go releases need Rust artifacts that don't exist yet
- Solution: Either create a Rust release first, or let the workflow build locally

## Build Commands

### Rust Library Build
```bash
# Build debug version (local development)
make build-debug

# Build release version with zigbuild
make build

# Build for all platforms (cross-compilation)
make build-all-targets

# Create release assets with checksums
make create-release-assets
```

### Testing
```bash
# Run full test suite with coverage (builds library first)
make test

# Run tests with specific library path
make test-lib-path

# Run tests for CI (expects library already built)
make test-ci

# Run Rust tests
make test-rust

# Test download functionality
make test-download

# Run a single Go test
go test -v -run TestFunctionName
```

### Linting
```bash
# Fix Go lint issues
make lint-fix

# Check Rust code with clippy
make lint-rust

# Format Rust code
make fmt-rust
```

## Architecture

### Library Loading Flow
The system follows a priority order for loading the tokenizer library:
1. User-provided path via `WithLibraryPath()` option
2. `TOKENIZERS_LIB_PATH` environment variable
3. Cached library in platform-specific directory
4. Automatic download from `releases.amikos.tech` (with GitHub Releases fallback) to cache

### Version Management
The project uses a single version from `Cargo.toml` for both the library and ABI compatibility:
- The `get_version()` function returns the Cargo package version (e.g., "0.1.0")
- This same version is used for ABI compatibility checking
- The Go side checks compatibility using the constraint `^0.1.x`
- When making breaking FFI changes, update the version in `Cargo.toml` following semantic versioning

### Core Components

**Go Layer (tokenizers.go)**
- Main `Tokenizer` struct managing FFI calls
- Encoding/decoding operations with configurable options
- Truncation and padding support
- ABI version compatibility checking

**HuggingFace Integration (huggingface.go)**
- Direct loading from HuggingFace model repository
- Authentication support for private/gated models
- Smart caching with offline mode support
- Retry logic with exponential backoff
- Model ID validation (owner/repo_name format, max 96 chars per component)

**FFI Bridge (library.go, library_windows.go)**
- Platform-specific library loading using purego
- No CGo dependencies - pure Go implementation

**Download System (download.go)**
- Automatic platform detection and asset selection
- Primary `releases.amikos.tech` endpoint with GitHub Releases fallback
- Checksum verification for all downloaded assets
- Intelligent caching in OS-appropriate directories

**Rust Layer (src/lib.rs)**
- C-compatible FFI exports using tokenizers crate
- Memory-safe buffer management
- Error code propagation

### Platform Support

The library detects and handles:
- **macOS**: x86_64, aarch64 (M1/M2) → .dylib files
- **Linux**: x86_64, aarch64 → .so files (gnu and musl variants)
- **Windows**: x86_64 → .dll files

### Cache Locations

#### Library Cache
- **macOS**: `~/Library/Caches/tokenizers/lib/`
- **Linux**: `~/.cache/tokenizers/lib/` or `$XDG_CACHE_HOME/tokenizers/lib/`
- **Windows**: `%APPDATA%/tokenizers/lib/`

#### HuggingFace Cache
- **macOS**: `~/Library/Caches/tokenizers/lib/hf/models/`
- **Linux**: `~/.cache/tokenizers/lib/hf/models/` or `$XDG_CACHE_HOME/tokenizers/lib/hf/models/`
- **Windows**: `%APPDATA%/tokenizers/lib/hf/models/`

For detailed cache structure and management, see `docs/CACHE_MANAGEMENT.md`.

## Environment Variables

- `TOKENIZERS_LIB_PATH`: Override library path
- `TOKENIZERS_VERSION`: Specific version to download (default: `latest`)
- `GITHUB_TOKEN` or `GH_TOKEN`: Optional GitHub authentication for fallback API requests
- `HF_TOKEN`: HuggingFace authentication token for private/gated models
- `HF_HUB_CACHE`: Override HuggingFace cache directory
- `HF_MAX_TOKENIZER_SIZE`: Maximum tokenizer file size in bytes (default: 524288000 / 500MB)
- `HF_HTTP_MAX_IDLE_CONNS`: Maximum number of idle HTTP connections (default: 100)
- `HF_HTTP_MAX_IDLE_CONNS_PER_HOST`: Maximum idle connections per host (default: 10)
- `HF_HTTP_IDLE_TIMEOUT`: Idle connection timeout duration, e.g., "2m30s" (default: 90s)

## Error Handling

The library uses numeric error codes defined in tokenizers.go:
- `SUCCESS (0)`: Operation successful
- `ErrInvalidUTF8 (-1)`: Invalid UTF-8 in input
- `ErrEncodingFailed (-2)`: Tokenization failed
- `ErrTokenizerCreationFailed (-6)`: Failed to create tokenizer
- Additional error codes for various failure scenarios

All errors are wrapped with context using `pkg/errors` for better debugging.

## Development Setup

```bash
# Install all required tools
make dev-setup

# Check environment configuration
make check-env

# Clean build artifacts and caches
make clean
```

## Key Implementation Details

- **ABI Compatibility**: The library version from `Cargo.toml` is used for compatibility checking (`AbiCompatibilityConstraint = "^0.1.x"`). The `get_version()` FFI function returns this version.
- **Memory Safety**: Proper cleanup of FFI resources with defer statements
- **Buffer Management**: Zero-copy where possible, explicit memory management for C strings
- **Cross-platform**: Uses runtime detection for platform-specific library names and paths
- Always lint both golang and rust before commiting or pushing code
