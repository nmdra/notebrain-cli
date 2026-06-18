# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A local Chroma runtime package with:
- Go wrapper API (purego, no cgo)
- Rust FFI shim
- Java scaffold bindings (`core`, `jna`, `panama`)

## Requirements

- Go 1.21+
- Rust 1.70+
- Java 17+ (JNA path), Java 22+ (Panama path)
- Gradle 9+
- golangci-lint (Go linting)

## Build Commands

```bash
make build          # Build Rust shim (debug)
make build-release  # Build Rust shim (release)
make test           # Build debug + run Go tests
make test-release   # Build release + run Go tests
make build-java     # Build Java modules (no tests)
make test-java      # Run Java smoke tests (JNA + Panama)
make test-all       # Go + Rust + Java smoke tests (Java skipped only if Gradle missing)
make lint           # Run all linters (Go + Rust)
make fmt            # Format all code (Go + Rust)
make clean          # Clean build artifacts
```

## Testing

Go tests require the Rust shim and are wired by Makefile:
- `make test` builds debug shim and runs Go tests
- `make test-release` builds release shim and runs Go tests
- `CHROMA_LIB_PATH` is auto-set by Makefile

Java smoke tests are available via:
- `make test-java` (runs `:jna:test` and `:panama:test`)
- `make test-all` fails on Java test failures when Gradle is present

## Architecture

```
Go Package (root)              Rust Shim (shim/)               Java scaffold (java/)
├── chroma.go      ─────────►  src/lib.rs (FFI exports)   ◄───┬── core (shared API models)
├── config.go                  (chroma_* symbols)             ├── jna (Java 17 fallback)
├── library.go                                                    └── panama (Java 22 primary)
└── errors.go
```

- **No cgo**: Uses purego for pure Go FFI
- **Runtime artifact name**: `chroma_shim` (`libchroma_shim.so`, `libchroma_shim.dylib`, `chroma_shim.dll`)
- **Configuration**: YAML-based embedded startup config
- **Resource cleanup**: explicit close semantics in Go and Java runtime/session wrappers, with Go finalizers as a fallback safety net

## Key Patterns

Builder pattern for configuration:
```go
server, err := chroma.NewServer(
    chroma.WithPort(8000),
    chroma.WithPersistPath("./chroma_data"),
)
```

YAML string config alternative:
```go
server, err := chroma.StartServer(chroma.StartServerConfig{
    ConfigString: yamlString,
})
```

## Linting

- Go: `golangci-lint run ./...` (config in `.golangci.yml`)
- Rust: `cargo clippy -- -D warnings` (warnings as errors)
- Java: `gradle --no-daemon :core:check :jna:check :panama:check`
