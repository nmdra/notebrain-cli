# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v2.0.0] - 2026-06-30

### Breaking Changes
- **Modernized JSON Schema (`snake_case`)**: All machine-readable structured outputs (`--format json`, `tsv`, `ndjson`) have been modernized from PascalCase (`Title`, `Score`, `FilePath`) to clean `snake_case` keys (`title`, `score`, `file_path`, `note_slug`, `tags`, `text`). Consumer automation scripts and AI agents must be updated to reference `snake_case` keys.

### Added
- **AI Agent Command Chaining (`--jsonpath`)**: Integrated `jsonpath` expression evaluation across all query and stats commands (`--jsonpath="$.results[0].note_slug"`). Scalar outputs format as unquoted raw strings and arrays print newline-separated elements, allowing direct shell pipeline integration without external JSON parsers like `jq`.
- **Complete Note Retrieval (`notebrain get`)**: Added a dedicated `get <slug-or-path>` command to retrieve and stitch together all indexed document chunks into the full reconstructed markdown note content.
- **Tag Search & Filtering (`--tag`)**: Added direct tag filtering (`--tag="TagName"`) to `notebrain search` and expanded tag extraction across note metadata.
- **AI Agent Skill Instructions**: Added and documented the built-in `notebrain-assistant` skill (`.agents/skills/notebrain/SKILL.md`) optimized for agentic coding tools.
- **TOML Configuration File Support**: Added support for persisting CLI flags via `~/.notebrain/config/config.toml` along with flags `--respect-exclude` and `--use-editor`.
- **External Editor Integration (`--use-editor`)**: Added ability to open matching notes directly in terminal/GUI editors defined by `$EDITOR` from the interactive TUI.
- **Obsidian Ignore & Attachment Filtering**: Automatically honors Obsidian's `userIgnoreFilters` and `attachmentFolderPath` settings during ingestion when `--respect-exclude` is enabled.

### Fixed
- **HNSW Concurrency & Integrity Bugs**: Implemented batch database writes and serialized chunk deletion/insertion operations to eliminate embedded `hnswlib` assertion failures during high concurrency ingestion and collection reset.

## [v1.1.0] - 2026-06-19

### Added
- **Beautiful TUI Integration**: Upgraded the CLI experience by integrating the `charm.land/bubbles v2` and `lipgloss v2` ecosystem.
- **Interactive Result Browser**: Semantic searches and link traversals now open an interactive terminal UI (TUI) where you can fuzzy-find through results, view scores, and instantly open matched notes in Obsidian using the `Enter` key.
- **Live Ingestion Progress**: Replaced the static progress output with a smooth, live-updating progress bar displaying exactly which files are being processed.

### Changed
- **CLI Framework Migration**: Completely migrated the CLI definition from `cobra` to `kong` for cleaner, declarative, struct-based command definitions, improving maintainability.
- **Safe Pipeline Interruptions**: You can now safely cancel the multi-worker ingestion pipeline at any time by pressing `ctrl+c` (or `q`/`esc` in the TUI). All workers will cleanly abort.

### Fixed
- **Concurrency Crash**: Fixed a critical bug where embedded `hnswlib` (ChromaDB) would crash with a core dump (assertion failure) during concurrent ingestion. Database writes are now safely synchronized with mutexes.
- **Missing FilePaths**: Fixed an issue where the `backlinks`, `connections`, `hidden`, and `tags` commands returned results missing `FilePath` metadata, meaning Obsidian URIs and the new interactive UI open feature now work perfectly for all commands.
- **Zombie Process Leak**: Fixed an issue where opening a note via the terminal leaked zombie processes in the background.
- **Chroma Path Resolution**: Fixed an issue where the `~` character in the database path was evaluated as a literal directory name instead of expanding to the user's home directory.



## [v1.0.0] - 2026-06-18

### Official Release
This marks the official stable release of **NoteBrain CLI v1.0.0**! This release successfully graduates all of our powerful experimental features from the alpha, beta, and release candidate channels into a robust, high-performance production version.

**Highlights of v1.0.0 include:**
- **Local Embedded AI**: Fully embedded `chroma-go` database and local ONNX embedding models right in the binary. No external servers needed.
- **AST-Aware Intelligence**: Goldmark-based markdown parsing for highly accurate, structure-aware semantic chunking.
- **Graph & Semantic Search Combined**: Search by vector similarity, explore wikilink connections (`--hops`), and run Graph-Boosted hybrid queries.
- **Terminal Integration**: Clickable OSC 8 `obsidian://open` terminal hyperlinks integrated natively.
- **Developer Experience**: Dotenv (`.env`) support, 74%+ test coverage, and automated `GoReleaser` pipelines.

## [v1.0.0-rc.1] - 2026-06-18

### Added
- **Goldmark AST-Aware Chunking**: Intelligently chunks markdown sections according to header hierarchies instead of arbitrary character splits, preserving code blocks and structural metadata.
- **Advanced Filtering**: Use `--section`, `--has-code`, and `--has-tasks` flags on searches to filter precisely by document structure.
- **OSC 8 Terminal Hyperlinks**: Automatically renders clickable `obsidian://open` links right in your CLI for seamlessly opening matched chunks inside Obsidian (supported terminals only). Added `--no-hyperlinks` flag to disable.
- **Environment Configuration**: Added `.env` file support (via `godotenv`) to manage global configuration like `OBSIDIAN_VAULT_PATH` and `OBSIDIAN_VAULT_NAME` without repetitive flags.

## [v1.0.0-beta] - 2026-06-18

### Added
- **Content Hashing**: Introduced SHA-256 hashing during the ingest pipeline to safely and instantly skip re-ingesting files that haven't changed.

### Changed
- **Performance**: Greatly improved test coverage (up to 74.1%) across parser, store, and ingest systems.
- **Refactoring**: Stripped over-engineered code: removed `obsidian` package, removed abstract embedder interfaces, and inlined custom sorting functions.

## [v1.0.0-alpha] - 2026-06-18

### Added
- **Embedded ChromaDB Engine**: Fully migrated from DuckDB/pgvector to an embedded `chroma-go` v2 vector database.
- **Local ONNX Embeddings**: Added in-process inference using local ONNX embedding models to vector-encode markdown chunks seamlessly.
- **Wikilink & Tag Graph Processing**: Parses Obsidian wikilinks and frontmatter tags to construct structural graph relationships in vector space.
- **CLI Commands**:
  - `ingest`: Fully concurrent pipeline to parse, chunk, and embed an Obsidian vault.
  - `search`: Semantic vector search for textual matching.
  - `backlinks`: Identifies incoming references to a target note via the structural graph.
  - `connections`: Explores breadth-first structural subgraphs (n-hop traversal).
  - `hidden`: Discovers "hidden" semantic links between unlinked notes based on high semantic proximity.
  - `boosted`: Combines vector similarity with graph connectivity (Graph-Boosted Semantic Search).
  - `tags`: Discovers notes sharing identical frontmatter tags.
  - `stats`: Analyzes current ChromaDB vector storage statistics.
  - `reset`: Completely purges the embedded vector database.
- **Automated CI/CD**: Added `GoReleaser` and GitHub Actions configuration for automated, cryptographically signed binary distribution and SBOM generation.
- **Documentation**: Comprehensive README and `wiki/` documentation covering Installation, Commands, and Architecture.
