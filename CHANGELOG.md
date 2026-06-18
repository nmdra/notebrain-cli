# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
