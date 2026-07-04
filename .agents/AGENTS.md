# NoteBrain CLI — Agent Rules

## Project Overview

NoteBrain is a Go CLI tool that indexes an Obsidian vault into ChromaDB for semantic search, backlink traversal, graph connections, hidden connections, shared tags, and graph-boosted search.

## Technology Stack

- **Language:** Go 1.26.3
- **CLI Framework:** [kong](https://github.com/alecthomas/kong) v1.15.x (with custom TOML configuration resolver)
- **Vector Store:** ChromaDB via [chroma-go](https://github.com/amikos-tech/chroma-go) v0.4.x (`pkg/api/v2`)
- **Build:** `CGO_ENABLED=1` (embedded persistent client via SQLite/HNSW bindings)

## Module Path

```
github.com/nmdra/notebrain-cli
```

## Project Structure

```
notebrain-cli/
├── main.go
├── go.mod
├── Makefile
├── config/
│   └── config.go
├── internal/
│   ├── store/         ← ChromaDB wrapper (replaces DuckDB)
│   │   ├── store.go
│   │   ├── upsert.go
│   │   ├── query.go
│   │   └── *_test.go
│   ├── parser/        ← Markdown parsing, slugify, chunking
│   │   ├── parser.go
│   │   └── *_test.go
│   ├── ingest/        ← File ingestion pipeline
│   │   ├── ingest.go
│   │   └── *_test.go
│   ├── embedder/      ← Embedding backends (MiniLM, Ollama)
│   │   ├── embedder.go
│   │   └── *_test.go
│   └── obsidian/      ← Obsidian CLI client
│       ├── client.go
│       └── *_test.go
└── cmd/
    ├── root.go
    ├── ingest.go
    ├── search.go
    ├── backlinks.go
    ├── connections.go
    ├── hidden.go
    ├── tags.go
    ├── boosted.go
    ├── stats.go
    └── reset.go
```

## Development Methodology

- **Test-Driven Development (TDD):** Write tests FIRST, then implement the minimum code to pass them.
- Every public function and method MUST have corresponding test coverage.
- Use table-driven tests where appropriate.
- Use `testify` for assertions only if already in go.mod; otherwise use stdlib `testing`.
- Name test files `*_test.go` alongside the source file.
- **Go Vendoring:** This repository uses Go vendoring (`vendor/`). Whenever dependencies in `go.mod` or `go.sum` are added, removed, or updated, you MUST run `go mod vendor` before running tests or builds.
- **Strict Non-Regression Guardrails:** When refactoring or removing features, always add explicit assertion tests across `config/`, `internal/configfile/`, and `internal/store/` to verify that existing core functions, default settings, TOML key resolution, and database initialization do not regress or depend on removed parameters.

## Coding Conventions

- Follow standard Go formatting (`gofmt`/`goimports`).
- Use `context.Context` as the first parameter for any function that does I/O.
- Errors must be wrapped with `fmt.Errorf("context: %w", err)` for traceability.
- No global mutable state. Pass dependencies explicitly (config, store, embedder).
- Use the options/functional-options pattern where it simplifies APIs.
- Keep packages focused: one responsibility per package.
- **TUI vs. Domain Package Boundaries:** The `internal/tui/` package is strictly reserved for interactive terminal user interface components (e.g., Bubble Tea models, interactive spinners). Non-interactive logging, progress tracking, and machine-readable output formatting must reside directly in their respective domain packages (e.g., `internal/ingest`), never in `internal/tui/`.

## ChromaDB Collections

| Collection | Purpose | Has Embeddings |
|---|---|---|
| `nb_chunks` | Note text chunks with vectors + metadata | Yes |
| `nb_links` | Wikilink graph edges (metadata-only) | No (dummy 1-dim) |

## Testing Strategy

1. **Unit tests** — Pure logic (parser, config, metadata encoding)
2. **Integration tests** — Store operations against a temporary ChromaDB (use `t.TempDir()`)
3. **No mocks for ChromaDB** — Use real persistent client in tests with temp directories

## Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):
```
feat(store): add UpsertChunks method
test(parser): add table-driven tests for Slugify
fix(ingest): handle empty frontmatter gracefully
```

## Linting

- Only lint services/directories that have changes in the working directory.
- Do NOT run all linting tasks at once.

## Key Design Decisions

1. Tags encoded as `tag_0`, `tag_1`, … (not array metadata) for Go client compatibility.
2. Graph traversal (BFS) done in Go, not SQL — ChromaDB has no SQL.
3. `nb_links` uses dummy 1-dim embeddings since Chroma requires uniform dimensions per collection.
4. `DeleteNoteChunks` BEFORE `UpsertChunks` (not after) to handle interrupted re-ingests.
5. Persistent client is single-writer — fine for CLI usage.
6. **TOML-Only Configuration:** Configuration hierarchy is strictly 2-tier: CLI flags > TOML configuration file (`~/.notebrain/config/config.toml` or `--config`). No `.env` files or application environment variable fallbacks are permitted. TOML keys support normalized matching (`snake_case` and `kebab-case` match interchangeably).
7. **Embedded Persistent Storage Only:** NoteBrain strictly embeds ChromaDB in persistent mode (`CGO_ENABLED=1`). Standalone HTTP server connections (`CGO_ENABLED=0`) are intentionally unsupported to keep the CLI lightweight, self-contained, and zero-setup.
8. **OS-Level Scheduled Ingestion:** In line with Unix philosophy, periodic re-indexing is handled by standard OS schedulers (cron, systemd timers) rather than a custom persistent background watch daemon or file-watching loop. Recommended ingestion interval is every 3 hours.
9. **Decoupled Automated Ingestion Logging (No TUI for Ingestion):** Because `notebrain ingest` is frequently executed as an automated background task (cron, systemd timers, agentic workflows), it strictly uses structured logging (`log/slog`) for progress reporting. Interactive TUI progress bars (e.g., Bubble Tea) are intentionally disabled and omitted from the ingestion pipeline to guarantee clean, machine-readable operational logs.
