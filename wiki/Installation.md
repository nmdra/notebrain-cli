# Installation

## Prerequisites
- Go 1.26.3 or higher.
- `make` (optional, for building from source).
- CGO enabled toolchain (GCC/Clang on Linux/macOS) since the embedded vector database requires C/C++ bindings.

## Installing via Pre-built Binaries (Recommended)

You can download the pre-built Linux or macOS binaries from the [GitHub Releases](https://github.com/nmdra/notebrain-cli/releases) page. Extract the archive and place the `notebrain` binary in your `$PATH`.

## Building from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/nmdra/notebrain-cli.git
   cd notebrain-cli
   ```

2. Build the binary using Make:
   ```bash
   make build
   ```
   *Note: This will execute `CGO_ENABLED=1 go build -o notebrain .`*

3. Move the binary to a directory in your PATH:
   ```bash
   sudo mv notebrain /usr/local/bin/
   ```

## Configuration

By default, NoteBrain stores the local ChromaDB database in your home directory:
```
~/.notebrain/chroma
```

You can change this using the global `--chroma-path` flag on any command.
