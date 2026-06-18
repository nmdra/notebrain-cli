# pure-tokenizers

[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24-blue.svg)](https://go.dev/)
[![CI Status](https://github.com/amikos-tech/pure-tokenizers/workflows/CI/badge.svg)](https://github.com/amikos-tech/pure-tokenizers/actions)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/amikos-tech/pure-tokenizers)](https://github.com/amikos-tech/pure-tokenizers/releases)

CGo-free tokenizers for Go with automatic library management and HuggingFace Hub integration.

- ✅ **No CGo required** - Pure Go implementation using purego FFI
- ✅ **HuggingFace Hub integration** - Load tokenizers directly from HuggingFace models
- ✅ **Automatic downloads** - Platform-specific libraries fetched on demand
- ✅ **Cross-platform** - Windows, macOS, Linux (including ARM)
- ✅ **Production ready** - Checksum verification and ABI compatibility checks

## Quick Start

### Load directly from HuggingFace Hub

```go
package main

import (
    "fmt"
    "log"

    "github.com/amikos-tech/pure-tokenizers"
)

func main() {
    // Load tokenizer directly from HuggingFace model
    tokenizer, err := tokenizers.FromHuggingFace("bert-base-uncased")
    if err != nil {
        log.Fatal(err)
    }
    defer tokenizer.Close()

    // Tokenize text
    encoding, err := tokenizer.Encode("Hello, world!", tokenizers.WithAddSpecialTokens())
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Tokens:", encoding.Tokens)
    fmt.Println("Token IDs:", encoding.IDs)
}
```

### Or load from a local file

```go
// Load tokenizer from file
tokenizer, err := tokenizers.FromFile("tokenizer.json")
if err != nil {
    log.Fatal(err)
}
defer tokenizer.Close()
```

That's it! The library automatically downloads the correct binary for your platform on first use.

## Installation

```bash
go get github.com/amikos-tech/pure-tokenizers
```

## Features

### 🚀 Zero Configuration
The library automatically manages platform-specific binaries. No manual downloads, no build steps, no CGo.

### 🔐 Secure by Default
- SHA256 checksum verification for all downloads
- ABI version compatibility checking
- Secure HTTPS-only downloads

### 🎯 Platform Native
Optimized binaries for each platform and architecture:
- macOS (Intel & Apple Silicon)
- Linux (x86_64, ARM64, including musl)
- Windows (x86_64)

### ⚡ High Performance
Native Rust performance without CGo overhead. Direct FFI calls using purego.

## Performance Benchmarks

The following benchmarks compare pure-tokenizers (CGo-free) with CGo-based implementations. Results show competitive performance while maintaining the benefits of a CGo-free approach.

### Benchmark Comparison

**Test Environment:**
- **pure-tokenizers**: Apple M3 Max, macOS (CGo-free implementation)
- **CGo baseline**: Apple M1 Pro, macOS ([daulet/tokenizers](https://github.com/daulet/tokenizers))

> **Note**: Different hardware affects absolute timings. Focus on relative performance patterns and memory characteristics rather than exact microsecond differences.

**Text Characteristics:**
- **Short**: <50 characters (typical word or phrase)
- **Medium**: 100-500 characters (typical sentence or paragraph)
- **Long**: >1000 characters (multiple paragraphs)

| Operation | Implementation | Time/op | Memory/op | Allocs/op | Notes |
|-----------|---------------|---------|-----------|-----------|--------|
| **Encode (Short Text)** | pure-tokenizers | 7.80μs | 920 B | 16 | CGo-free |
| | CGo baseline | 10.50μs | 256 B | 12 | HuggingFace tokenizer |
| **Encode (Medium Text)** | pure-tokenizers | 30.50μs | 1,552 B | 35 | CGo-free |
| **Encode (Long Text)** | pure-tokenizers | 267.00μs | 6,864 B | 165 | CGo-free |
| **Decode Operations** | pure-tokenizers | 13.40μs | 740 B | 10 | CGo-free |
| | CGo baseline | 1.50μs | 64 B | 2 | HuggingFace tokenizer |
| **Encode/Decode Cycle** | pure-tokenizers | 52.50μs | 2,296 B | 45 | Medium text, CGo-free |

### Key Performance Characteristics

**✅ Advantages of CGo-free approach:**
- **No CGo overhead**: Eliminates C-Go boundary crossing costs
- **Cross-compilation friendly**: No CGo dependencies simplify building
- **Memory safety**: Pure Go memory management
- **Deployment simplicity**: Single binary with automatic library management

**📊 Performance Analysis:**
- **Encoding performance**: Competitive with CGo implementations, often faster for short texts
- **Memory usage**: Higher allocation count due to FFI boundary (16 vs 12 allocs), but predictable patterns
- **Batch processing**: Efficient handling of multiple text inputs
- **Platform consistency**: Consistent performance across all supported platforms

### Advanced Benchmarks

| Feature | Time/op | Memory/op | Allocs/op | Notes |
|---------|---------|-----------|-----------|-------|
| **Batch Processing** (5 texts) | 356.00μs | 11,568 B | 261 | Parallel encoding |
| **With Options** (all attributes) | 34.30μs | 2,160 B | 41 | Full feature set |
| **Truncation** (128 tokens) | 258.00μs | 5,632 B | 127 | Max length enforcement |
| **Padding** (256 tokens) | 84.90μs | 16,272 B | 535 | Fixed length output |
| **HuggingFace Loading** (cached) | 26.20ms | 6.45 MB | 92,188 | Model initialization |

### Benchmark Environment

```bash
# Run benchmarks locally
make build && go test -bench=. -benchmem

# Compare with different tokenizers
go test -bench=BenchmarkEncode -benchmem
go test -bench=BenchmarkDecode -benchmem
```

**Platform-specific results**: Benchmarks run continuously in CI across Linux, macOS, and Windows. See [benchmark workflow](.github/workflows/benchmark.yml) for automated performance tracking.

## Usage Examples

### HuggingFace Hub Integration

```go
// Load tokenizer from any public HuggingFace model
tokenizer, err := tokenizers.FromHuggingFace("bert-base-uncased")
tokenizer, err := tokenizers.FromHuggingFace("gpt2")
tokenizer, err := tokenizers.FromHuggingFace("sentence-transformers/all-MiniLM-L6-v2")

// Load from private/gated models with authentication
tokenizer, err := tokenizers.FromHuggingFace("meta-llama/Llama-2-7b-hf",
    tokenizers.WithHFToken(os.Getenv("HF_TOKEN")))

// Configure HuggingFace options
tokenizer, err := tokenizers.FromHuggingFace("bert-base-uncased",
    tokenizers.WithHFToken(token),           // Authentication token
    tokenizers.WithHFRevision("main"),       // Specific revision/branch
    tokenizers.WithHFCacheDir("/custom/cache"), // Custom cache directory
    tokenizers.WithHFTimeout(30*time.Second),   // Download timeout
    tokenizers.WithHFOfflineMode(true),      // Use cached version only
)

// The tokenizer is automatically cached for offline use
// Cache location: ~/.cache/tokenizers/huggingface/ (Linux/macOS)
//                 %APPDATA%/tokenizers/huggingface/ (Windows)
```

📚 **See also:**
- [HuggingFace Integration Guide](docs/HUGGINGFACE.md) - Comprehensive documentation
- [Example: Basic Usage](examples/huggingface/basic/) - Loading various models
- [Example: Cache Management](examples/huggingface/cache/) - Working with cache
- [Example: Private Models](examples/huggingface/private/) - Authentication and gated models

### Basic Tokenization

```go
// Load a tokenizer from file
tokenizer, err := tokenizers.FromFile("path/to/tokenizer.json")
if err != nil {
    log.Fatal(err)
}
defer tokenizer.Close()

// Simple encoding
encoding, err := tokenizer.Encode("Hello, world!")

// With special tokens
encoding, err := tokenizer.Encode("Hello, world!", tokenizers.WithAddSpecialTokens())
```

### Advanced Options

```go
// Encoding with custom options
encoding, err := tokenizer.Encode("Your text here",
    tokenizers.WithAddSpecialTokens(),
    tokenizers.WithReturnTokens(),
    tokenizers.WithReturnAttentionMask(),
    tokenizers.WithReturnTypeIDs(),
)

// Create tokenizer with truncation and padding
tokenizer, err := tokenizers.FromFile("tokenizer.json",
    tokenizers.WithTruncation(512, tokenizers.TruncationDirectionRight, tokenizers.TruncationStrategyLongestFirst),
    tokenizers.WithPadding(true, tokenizers.PaddingStrategy{Tag: tokenizers.PaddingStrategyFixed, FixedSize: 512}),
)

// Access different parts of the encoding result
if encoding.Tokens != nil {
    fmt.Println("Tokens:", encoding.Tokens)
}
if encoding.IDs != nil {
    fmt.Println("Token IDs:", encoding.IDs)
}
if encoding.AttentionMask != nil {
    fmt.Println("Attention mask:", encoding.AttentionMask)
}
```

### Decoding Tokens

```go
// Decode token IDs back to text
ids := []uint32{101, 7592, 1010, 2088, 999, 102}
text, err := tokenizer.Decode(ids, true)
fmt.Println(text)  // "hello, world!"
```

### Loading from Configuration Files

```go
// Load tokenizer from a downloaded tokenizer.json file
tokenizer, err := tokenizers.FromFile("path/to/tokenizer.json")

// Load from byte configuration
configBytes, _ := os.ReadFile("tokenizer.json")
tokenizer, err := tokenizers.FromBytes(configBytes)

// Use with custom library path
tokenizer, err := tokenizers.FromFile("tokenizer.json",
    tokenizers.WithLibraryPath("/custom/path/to/libtokenizers.so"))
```

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TOKENIZERS_LIB_PATH` | Custom library path | Auto-detect |
| `TOKENIZERS_VERSION` | Library version to download | `latest` |
| `GITHUB_TOKEN` / `GH_TOKEN` | Optional token for GitHub API/authenticated fallback requests | unset |

### Library Loading Options

```go
// Use a specific library path
tokenizer, err := tokenizers.FromFile("tokenizer.json",
    tokenizers.WithLibraryPath("/custom/path/to/libtokenizers.so"))

// The library loading priority:
// 1. User-provided path via WithLibraryPath()
// 2. TOKENIZERS_LIB_PATH environment variable
// 3. Cached library in platform directory
// 4. Automatic download from releases.amikos.tech (with GitHub Releases fallback)
```

### Cache Management

For comprehensive cache management documentation, see [Cache Management Guide](docs/CACHE_MANAGEMENT.md).

#### Library Cache
```go
// Get the library cache directory
cachePath := tokenizers.GetCachedLibraryPath()

// Clear the library cache
err := tokenizers.ClearLibraryCache()

// Download and cache a specific version
err := tokenizers.DownloadAndCacheLibraryWithVersion("v0.1.0")

// Discover release versions
versions, err := tokenizers.GetAvailableVersions()
// Note: current release metadata exposes only latest.json,
// so this returns at most one latest version.
```

#### HuggingFace Cache
```go
// Get HuggingFace cache information
info, err := tokenizers.GetHFCacheInfo("bert-base-uncased")

// Clear cache for a specific model
err := tokenizers.ClearHFModelCache("bert-base-uncased")

// Clear entire HuggingFace cache
err := tokenizers.ClearHFCache()

// Use offline mode (only use cached tokenizers)
tokenizer, err := tokenizers.FromHuggingFace("bert-base-uncased",
    tokenizers.WithHFOfflineMode(true))
```

HuggingFace cache directories are created with `0750` permissions, and cached
`tokenizer.json` files are written with `0600` (owner read/write only).

## Platform Support

| Platform | Architecture | Binary | Status |
|----------|-------------|--------|--------|
| macOS | x86_64 | `.dylib` | ✅ |
| macOS | aarch64 (M1/M2) | `.dylib` | ✅ |
| Linux | x86_64 | `.so` | ✅ |
| Linux | aarch64 | `.so` | ✅ |
| Linux (musl) | x86_64 | `.so` | ✅ |
| Linux (musl) | aarch64 | `.so` | ✅ |
| Windows | x86_64 | `.dll` | ✅ |

## Development

### Building from Source

```bash
# Install Rust (if not already installed)
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh

# Clone the repository
git clone https://github.com/amikos-tech/pure-tokenizers
cd pure-tokenizers

# Build the Rust library
make build

# Run tests
make test

# Run linting
make lint-fix      # Go linting
make lint-rust     # Rust linting

# Run security checks
make security-go   # go vet + govulncheck + gosec
```

### Security Checks

```bash
# Run all Go security checks locally
make security-go

# Install and enable git hooks (lefthook)
go install github.com/evilmartians/lefthook@latest
lefthook install
```

### Testing

#### Unit Tests
```bash
# Run all unit tests
make test

# Run with specific library path
make test-lib-path
```

#### Integration Tests
Integration tests verify real-world functionality with HuggingFace models:

```bash
# Setup for local testing
cp .env.example .env
# Edit .env and add your HF_TOKEN (get from https://huggingface.co/settings/tokens)

# Run all integration tests (requires HF_TOKEN for private models)
make test-integration

# Run only HuggingFace integration tests
make test-integration-hf
```

The integration tests cover:
- Public model downloads (BERT, GPT2, DistilBERT)
- Private model access (with HF_TOKEN)
- Caching behavior verification
- Rate limiting handling
- Offline mode functionality

Note: Integration tests are automatically run in CI for the main branch and PRs with the `integration` label.

### Project Structure

```
pure-tokenizers/
├── src/           # Rust FFI implementation
├── *.go           # Go bindings
├── download.go    # Auto-download functionality
├── library.go     # Platform-specific FFI loading
└── Makefile       # Build automation
```

### Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

Built on top of the excellent [Hugging Face Tokenizers](https://github.com/huggingface/tokenizers) library.
