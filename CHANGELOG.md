# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
