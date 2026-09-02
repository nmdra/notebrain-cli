# Installation

## Prerequisites

- Go 1.26.4 or higher.
- `make` (optional, to build from source).
- A CGO-enabled toolchain (GCC or Clang on Linux or macOS). The embedded vector database requires C and C++ bindings.

## Install Pre-built Binaries (Recommended)

1. Download the pre-built Linux binary from the [GitHub Releases](https://github.com/nmdra/notebrain-cli/releases) page.
2. Extract the archive.
3. Put the `notebrain` binary in your `$PATH`.

Note: Pre-built macOS and Windows binaries are not shipped. To use macOS or Windows, build the binary from the source code.

## Build from Source

1. Clone the repository:
   ```bash
   git clone https://github.com/nmdra/notebrain-cli.git
   cd notebrain-cli
   ```

2. Use Make to build the binary:
   ```bash
   make build
   ```
   Note: This command executes `CGO_ENABLED=1 go build -o notebrain .`

3. Move the binary to a directory in your `$PATH`:
   ```bash
   sudo mv notebrain /usr/local/bin/
   ```

## Configuration

NoteBrain uses a TOML file for persistent configuration. The default location is `~/.notebrain/config/config.toml`.

1. Create the configuration directory:
   ```bash
   mkdir -p ~/.notebrain/config
   ```

2. Copy the template from the repository:
   ```bash
   cp config.example.toml ~/.notebrain/config/config.toml
   ```

3. Edit `~/.notebrain/config/config.toml`. Set your vault path, the database storage location, and the default format:
   ```toml
   vault-path = "/path/to/your/Obsidian Vault"
   vault-name = "My Vault"
   chroma-path = "~/.notebrain/chroma"
   format = "text"
   ```

By default, NoteBrain stores the local ChromaDB database at `~/.notebrain/chroma`. You can override any setting with command-line flags (for example, `--chroma-path`, `--vault-path`, `--format`).

## ONNX Model Recovery

Semantic commands use the local `all-MiniLM-L6-v2` model. The model cache is:

```text
~/.cache/chroma/onnx_models/all-MiniLM-L6-v2/onnx/
```

The cache must contain these non-empty files:

```text
model.onnx
tokenizer.json
```

Run the diagnostic command before you run a semantic command:

```bash
notebrain doctor
```

If the model is missing, download and verify the pinned Chroma archive:

```bash
archive=/tmp/notebrain-onnx.tar.gz
url=https://chroma-onnx-models.s3.amazonaws.com/all-MiniLM-L6-v2/onnx.tar.gz
curl -fL "$url" -o "$archive"
printf '%s  %s\n' \
  913d7300ceae3b2dbc2c50d1de4baacab4be7b9380491c27fab7418616a16ec3 \
  "$archive" | sha256sum -c -
model_dir=~/.cache/chroma/onnx_models/all-MiniLM-L6-v2/onnx
mkdir -p "$model_dir"
tar -xzf "$archive" -C "$model_dir"
notebrain doctor
```

The archive URL and checksum are in `vendor/github.com/amikos-tech/chroma-go/pkg/embeddings/default_ef/download_utils.go`.

If NoteBrain reports an active download lock, wait for the process to finish. Do not remove the lock.

If NoteBrain reports a stale lock, confirm that the reported process ID is not running. Then remove only the lock file and run `notebrain doctor` again:

```bash
rm ~/.cache/chroma/onnx_models/.download.lock
notebrain doctor
```

Do not remove the ChromaDB directory. The lock file and model cache are separate from the indexed vault data.
