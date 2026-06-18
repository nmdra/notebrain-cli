# AGENTS.md

Guidance for coding agents working in this repository.

## Project Summary

- `local-go-chroma` is a Go wrapper for running Chroma as an embedded server.
- It uses a Rust FFI shim plus `purego` (no `cgo`).
- Primary languages: Go, Rust, and Java.

## Requirements

- Go 1.21+
- Rust 1.70+
- Java 17+ (JNA) and Java 22+ (Panama)
- `golangci-lint` for Go linting

## Common Commands

- Build debug shim: `make build`
- Build release shim: `make build-release`
- Build Java modules: `make build-java`
- Run tests (debug): `make test`
- Run tests (release): `make test-release`
- Run Java smoke tests: `make test-java`
- Run linters: `make lint`
- Format code: `make fmt`

Notes:
- `make test` and `make test-release` set `CHROMA_LIB_PATH` automatically.
- Prefer Make targets over ad-hoc commands for reproducibility.

## Code Map

- `chroma.go`: server lifecycle and public Go API
- `rebuild.go`: collection rebuild maintenance API and server orchestration
- `config.go`: server config and builder options (`With...`)
- `library.go`: dynamic library loading and symbol binding via `purego`
- `errors.go`: error handling types and codes
- `shim/src/lib.rs`: Rust FFI exports and runtime-backed server operations
- `chroma_test.go`: integration-style tests against real server instances
- `java/core`: shared Java runtime API surface
- `java/jna`: Java 17 JNA bindings
- `java/panama`: Java 22 Panama bindings

## Implementation Rules

- Preserve the no-`cgo` design.
- Keep Go and Rust FFI contracts in sync when changing signatures.
- Maintain resource cleanup behavior (`Stop`, `Close`, and finalizers).
- Keep public API changes backward compatible unless explicitly requested.
- Add or update tests for behavior changes.
- Prefer public API call sites in functional-option form (`WithX(...)`) over nested option structs when introducing or refactoring APIs.
- Validate functional options in the entrypoint method by looping over all provided options and returning clear errors before any side effects.

## Validation Before Handoff

- Run relevant checks for touched areas:
- `make test`
- `make lint`

If a full run is not possible, document exactly what was not executed and why.
