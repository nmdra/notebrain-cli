.PHONY: build build-debug build-release build-java clean test test-go test-rust test-java test-all bench bench-go bench-rust lint lint-go lint-rust lint-java fmt fmt-go fmt-rust help

UNAME_S := $(shell uname -s 2>/dev/null || echo UNKNOWN)
OS_ENV := $(OS)

ifeq ($(UNAME_S),UNKNOWN)
$(warning uname -s failed; OS detection will rely on OS env and fallback rules)
endif

ifeq ($(OS_ENV),Windows_NT)
    HOST_OS := windows
else ifneq (,$(findstring MINGW,$(UNAME_S)))
    HOST_OS := windows
else ifneq (,$(findstring MSYS,$(UNAME_S)))
    HOST_OS := windows
else ifneq (,$(findstring CYGWIN,$(UNAME_S)))
    HOST_OS := windows
else ifeq ($(UNAME_S),Darwin)
    HOST_OS := darwin
else ifeq ($(UNAME_S),Linux)
    HOST_OS := linux
else ifeq ($(UNAME_S),UNKNOWN)
    HOST_OS := linux
else
    HOST_OS := linux
$(warning unrecognized uname -s '$(UNAME_S)'; defaulting HOST_OS to linux)
endif

ifeq ($(HOST_OS),darwin)
    LIB_EXT := dylib
    LIB_NAME := libchroma_shim.dylib
else ifeq ($(HOST_OS),windows)
    LIB_EXT := dll
    LIB_NAME := chroma_shim.dll
else
    LIB_EXT := so
    LIB_NAME := libchroma_shim.so
endif

SHIM_DIR := shim
CARGO_TARGET_DIR_ENV := $(strip $(CARGO_TARGET_DIR))

ifeq ($(CARGO_TARGET_DIR_ENV),)
    SHIM_TARGET_DIR := $(SHIM_DIR)/target
else ifneq ($(filter /%,$(CARGO_TARGET_DIR_ENV)),)
    SHIM_TARGET_DIR := $(CARGO_TARGET_DIR_ENV)
else ifneq ($(findstring :,$(CARGO_TARGET_DIR_ENV)),)
    SHIM_TARGET_DIR := $(CARGO_TARGET_DIR_ENV)
else ifneq ($(findstring \\,$(CARGO_TARGET_DIR_ENV)),)
    SHIM_TARGET_DIR := $(CARGO_TARGET_DIR_ENV)
else
    SHIM_TARGET_DIR := $(SHIM_DIR)/$(CARGO_TARGET_DIR_ENV)
endif

SHIM_TARGET_DEBUG := $(SHIM_TARGET_DIR)/debug/$(LIB_NAME)
SHIM_TARGET_RELEASE := $(SHIM_TARGET_DIR)/release/$(LIB_NAME)
JAVA_DIR := java
JAVA_GRADLE ?= gradle

ifeq ($(HOST_OS),windows)
VERIFY_DEBUG_ARTIFACT := @echo "Skipping POSIX artifact check on Windows Make host; use scripts/dev-windows.ps1 for artifact verification."
VERIFY_RELEASE_ARTIFACT := @echo "Skipping POSIX artifact check on Windows Make host; use scripts/dev-windows.ps1 for artifact verification."
RUN_GO_TEST_DEBUG := @echo "Windows Make host detected; use: pwsh -File .\\scripts\\dev-windows.ps1 -Task test" && exit 1
RUN_GO_TEST_RELEASE := @echo "Windows Make host detected; use: pwsh -File .\\scripts\\dev-windows.ps1 -Task test-release" && exit 1
RUN_GO_BENCH_DEBUG := @echo "Windows Make host detected; use: pwsh -File .\\scripts\\dev-windows.ps1 -Task bench-go" && exit 1
else
VERIFY_DEBUG_ARTIFACT := @test -f "$(SHIM_TARGET_DEBUG)" || (echo "Expected debug library not found at $(SHIM_TARGET_DEBUG). Check CARGO_TARGET_DIR." && exit 1)
VERIFY_RELEASE_ARTIFACT := @test -f "$(SHIM_TARGET_RELEASE)" || (echo "Expected release library not found at $(SHIM_TARGET_RELEASE). Check CARGO_TARGET_DIR." && exit 1)
RUN_GO_TEST_DEBUG := CHROMA_LIB_PATH=$(abspath $(SHIM_TARGET_DEBUG)) go test -v ./...
RUN_GO_TEST_RELEASE := CHROMA_LIB_PATH=$(abspath $(SHIM_TARGET_RELEASE)) go test -v ./...
RUN_GO_BENCH_DEBUG := CHROMA_LIB_PATH=$(abspath $(SHIM_TARGET_DEBUG)) go test -run '^$$' -bench . -benchmem ./...
endif

help:
	@echo "Chroma Go Shim Build System"
	@echo ""
	@echo "Available targets:"
	@echo "  build         - Build the Rust shim in debug mode"
	@echo "  build-debug   - Build the Rust shim in debug mode"
	@echo "  build-release - Build the Rust shim in release mode"
	@echo "  test          - Run Go tests (requires debug build)"
	@echo "  test-go       - Run Go tests (requires debug build)"
	@echo "  test-rust     - Run Rust shim tests"
	@echo "  test-all      - Run Go, Rust, and Java smoke tests (Java skipped if Gradle missing)"
	@echo "  test-release  - Run Go tests with release build"
	@echo "  bench         - Run Go and Rust benchmarks"
	@echo "  bench-go      - Run Go benchmarks"
	@echo "  bench-rust    - Run Rust criterion benchmarks"
	@echo "  build-java    - Build Java modules (core, jna, panama)"
	@echo "  test-java     - Run Java smoke tests (JNA + Panama)"
	@echo "  lint-java     - Run Java checks"
	@echo "  lint          - Run linters for Go and Rust"
	@echo "  lint-go       - Run golangci-lint"
	@echo "  lint-rust     - Run cargo clippy"
	@echo "  fmt           - Format Go and Rust code"
	@echo "  fmt-go        - Format Go code with gofmt and goimports"
	@echo "  fmt-rust      - Format Rust code with cargo fmt"
	@echo "  clean         - Clean build artifacts"
	@echo ""
	@echo "Environment variables:"
	@echo "  CHROMA_LIB_PATH - Path to the shared library (auto-set during tests)"
	@echo ""
	@echo "Windows (PowerShell):"
	@echo "  pwsh -File .\\scripts\\dev-windows.ps1 -Task test"
	@echo "  pwsh -File .\\scripts\\dev-windows.ps1 -Task lint"

build: build-debug

build-debug:
	cd $(SHIM_DIR) && cargo build --locked
	$(VERIFY_DEBUG_ARTIFACT)
	@echo "Built debug library at $(SHIM_TARGET_DEBUG)"

build-release:
	cd $(SHIM_DIR) && cargo build --locked --release
	$(VERIFY_RELEASE_ARTIFACT)
	@echo "Built release library at $(SHIM_TARGET_RELEASE)"

build-java:
	@set -e; \
	if [ ! -d "$(JAVA_DIR)" ]; then \
		echo "Missing $(JAVA_DIR) directory"; \
		exit 1; \
	elif ! command -v $(JAVA_GRADLE) >/dev/null 2>&1; then \
		echo "Gradle not found; skipping Java build"; \
	else \
		cd $(JAVA_DIR) && $(JAVA_GRADLE) --no-daemon :core:build :jna:build :panama:build -x test; \
	fi

test: test-go

test-go: build-debug
	$(RUN_GO_TEST_DEBUG)

test-rust:
	cd $(SHIM_DIR) && cargo test --locked

test-java: build-debug
	@set -e; \
	if [ ! -d "$(JAVA_DIR)" ]; then \
		echo "Missing $(JAVA_DIR) directory"; \
		exit 1; \
	elif ! command -v $(JAVA_GRADLE) >/dev/null 2>&1; then \
		echo "Gradle not found; skipping Java tests"; \
	else \
		cd $(abspath $(JAVA_DIR)) && \
		CHROMA_LIB_PATH=$(abspath $(SHIM_TARGET_DEBUG)) $(JAVA_GRADLE) --no-daemon :jna:test && \
		CHROMA_LIB_PATH=$(abspath $(SHIM_TARGET_DEBUG)) $(JAVA_GRADLE) --no-daemon :panama:test; \
	fi

test-all: test-go test-rust
	$(MAKE) test-java

test-release: build-release
	$(RUN_GO_TEST_RELEASE)

bench: bench-go bench-rust

bench-go: build-debug
	$(RUN_GO_BENCH_DEBUG)

bench-rust:
	cd $(SHIM_DIR) && cargo bench --locked --bench ffi_bench

clean:
	cd $(SHIM_DIR) && cargo clean
	rm -rf ./chroma_test_data

lint: lint-go lint-rust

lint-go:
	golangci-lint run ./...

lint-rust:
	cd $(SHIM_DIR) && cargo clippy --locked -- -D warnings

lint-java:
	@set -e; \
	if [ ! -d "$(JAVA_DIR)" ]; then \
		echo "Missing $(JAVA_DIR) directory"; \
		exit 1; \
	elif ! command -v $(JAVA_GRADLE) >/dev/null 2>&1; then \
		echo "Gradle not found; skipping Java lint"; \
	else \
		cd $(JAVA_DIR) && $(JAVA_GRADLE) --no-daemon :core:check :jna:check :panama:check; \
	fi

fmt: fmt-go fmt-rust

fmt-go:
	gofmt -w .
	goimports -w .

fmt-rust:
	cd $(SHIM_DIR) && cargo fmt
