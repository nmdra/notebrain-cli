# Installation

## Prerequisites
- Go 1.26.4 or higher.
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

NoteBrain uses a TOML file for persistent configuration (defaults to `~/.notebrain/config/config.toml`).

1. Create your configuration directory:
   ```bash
   mkdir -p ~/.notebrain/config
   ```
2. Copy the template from the repository:
   ```bash
   cp config.example.toml ~/.notebrain/config/config.toml
   ```
3. Edit `~/.notebrain/config/config.toml` to set your vault path, database storage location, and default formatting:
   ```toml
   vault-path = "/path/to/your/Obsidian Vault"
   vault-name = "My Vault"
   chroma-path = "~/.notebrain/chroma"
   format = "text"
   ```

By default, NoteBrain stores the local ChromaDB database at `~/.notebrain/chroma`. You can override any setting using command-line flags (e.g., `--chroma-path`, `--vault-path`, `--format`).
