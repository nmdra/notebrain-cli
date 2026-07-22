# NoteBrain CLI

A Go CLI tool that turns your [Obsidian](https://obsidian.md/) vault into a fully offline knowledge backend for **AI coding agents**. NoteBrain indexes markdown notes into a local **[ChromaDB](https://www.trychroma.com/)** vector database and exposes semantic search, wikilink graph traversal, and hidden connection discovery through structured output — designed to be chained directly by autonomous agents, shell pipelines, and LLM tool-use workflows.

Ships with an [AI agent skill](wiki/Skill_Usage.md) and [OpenCode Agent Configuration](wiki/OpenCode_Integration.md) for integration with autonomous coding agents like [OpenCode](https://opencode.ai), [Pi agent](https://pi.dev), and Claude Code. This setup is specially optimized to reduce token usage and latency.

[![Release](https://github.com/nmdra/notebrain-cli/actions/workflows/release.yml/badge.svg)](https://github.com/nmdra/notebrain-cli/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/nmdra/notebrain-cli.svg)](https://pkg.go.dev/github.com/nmdra/notebrain-cli/v2)
[![Ask DeepWiki](https://deepwiki.com/badge.svg)](https://deepwiki.com/nmdra/notebrain-cli)
[![Go Version](https://img.shields.io/github/go-mod/go-version/nmdra/notebrain-cli)](https://github.com/nmdra/notebrain-cli/blob/master/go.mod)
[![License: MIT](https://img.shields.io/github/license/nmdra/notebrain-cli)](https://github.com/nmdra/notebrain-cli/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/nmdra/notebrain-cli)](https://github.com/nmdra/notebrain-cli/releases)
[![GitHub stars](https://img.shields.io/github/stars/nmdra/notebrain-cli?style=social)](https://github.com/nmdra/notebrain-cli/stargazers)

<p align="center">
  <img src="assets/banner.svg" alt="NoteBrain CLI — AI-powered knowledge backend for your Obsidian vault" width="100%">
</p>

> [!NOTE]
> **Hi, I'm [Nimendra](https://nimendra.xyz).**  
> I use [Obsidian](https://obsidian.md/) daily as my primary note-taking solution. When AI agents emerged, I wanted to use my Obsidian vault as an RAG system.But most existing solutions don't fulfill my requirements.  
> While researching, I came across [this article](https://motherduck.com/blog/obsidian-rag-duckdb-motherduck/), which inspired this project.So I built this for my personal use. While you can use it directly, I highly encourage you to fork and modify this solution for your own use case.
>
> > _I don't use Windows or macOS, so those versions aren't shipped directly, but you can compile the binary using the source code._

## Features

- **Semantic Search** — Find notes by meaning, not just keywords, using the offline `all-MiniLM-L6-v2` ONNX embedding model.
- **Multi-Query Search** — Search with multiple independent queries to improve retrieval for complex topics and AI agent workflows.
- **Knowledge Graph Traversal** — Explore your Obsidian wikilink graph through backlinks, multi-hop connections, and shared tag relationships.
- **Hidden Connections** — Discover semantically related notes that aren't explicitly linked, with optional deep section-level analysis.
- **Graph-Boosted Ranking** — Improve search relevance by combining semantic similarity with graph relationships.
- **Advanced Filtering** — Refine results by sections, tags, code blocks, tasks, and other note metadata.
- **Full Note Retrieval** — Reconstruct complete notes on demand from indexed content.
- **Structured Output** — Export results as JSON or TSV, with built-in JSONPath querying for easy automation.
- **AI Agent Integration** — Includes a built-in AI agent skill and dedicated for autonomous knowledge retrieval.
- **Terminal Hyperlinks** — Open notes directly from supported terminals using OSC 8 hyperlinks.
- **Obsidian-Aware Indexing** — Respects your Obsidian configuration, including ignored files, attachment folders, and optional exclusion of empty-note references.

> _Note: Currently, this tool focuses on Markdown text only and does not support PDF or image OCR._

### Under the Hood

- **Goldmark AST-Aware Chunking** — Splits markdown by header hierarchy rather than arbitrary character offsets, strictly preserving lists, GFM tables, blockquotes/callouts, and code blocks.
- **Embedded ChromaDB** — Stores vectors directly on disk via [`chroma-go`](https://github.com/amikos-tech/chroma-go).
- **Incremental Ingestion** — SHA-256 content hashing skips unmodified notes in milliseconds on re-runs.

> _See the [Architecture](wiki/Architecture.md) guide for more details._

## Prerequisites

- **Go 1.26.4+**
- **CGO-enabled toolchain**
- Linux (macOS and Windows binaries are untested)

## Installation

Download a pre-built binary from the [GitHub Releases](https://github.com/nmdra/notebrain-cli/releases) page, or build from source:

```bash
git clone https://github.com/nmdra/notebrain-cli.git
cd notebrain-cli
make build          # CGO_ENABLED=1 go build -o notebrain .
sudo mv notebrain /usr/local/bin/
```

See the full [Installation Guide](wiki/Installation.md) for details.

## Quick Start

**1. Index your vault:**

```bash
notebrain ingest --vault-path "/path/to/your/Obsidian Vault"
```

> _Note: First-time indexing may take several minutes depending on your vault size._

**2. Search your notes by meaning:**

```bash
notebrain search "how do message brokers work?" --limit 5 --top-k 2
```

<p align="center">
  <img src="assets/search.png" alt="Notebrain search" width="100%">
</p>

**3. Discover deep hidden connections across note sections:**

Find notes that share similar concepts without direct wikilinks, using `--deep` chunk-by-chunk section matching (`§ <Heading>`):

```bash
notebrain hidden "TLS" --deep
```

<p align="center">
  <img src="assets/deep-hidden-connections.png" alt="Notebrain deep hidden connections" width="100%">
</p>

**4. Get structured output for scripts and AI agents:**

```bash
notebrain search "how do message brokers work?" --limit 2 --top-k 1 --format=json | jq
```

<details>
<summary>Example JSON output</summary>

<p align="center">
  <img src="assets/search-json.png" alt="Notebrain search JSON" width="100%">
</p>

</details>

**5. Chain commands to retrieve full notes:**

```bash
# Extract slug from top search result
SLUG=$(notebrain search "message broker" --limit 1 --jsonpath="$.results[0].note_slug")

# Retrieve complete reconstructed note text
notebrain get "$SLUG" --jsonpath="$.text"
```

**6. Automate indexing** with a cron job or systemd timer so your index stays fresh (see [Scheduled Ingestion](wiki/Scheduled_Ingestion.md)).

**7. Integration with AI Agents**

Use the built-in [AI agent skill](wiki/Skill_Usage.md) and [OpenCode Agent Configuration](wiki/OpenCode_Integration.md) for knowledge retrieval.

> [!tip]
> I highly recommend using the [Pi Agent](wiki/Pi_Agent.md) with the provided skill. It delivers higher-quality results, even with low cost models such as [DeepSeek V4 Flash](https://www.deepseek.com/) / [tencent hy3](https://hy.tencent.com/) / [Gemini Flash 3.6](https://ai.google.dev/gemini-api/docs/flash), without consuming unnecessary tokens. It also improves cache hit rates, helping reduce overall costs.
>
> For LLM models, use the **medium / low** thinking mode for fast responses.

[![asciicast](https://asciinema.org/a/1261133.svg)](https://asciinema.org/a/1261133)

## Configuration

NoteBrain reads configuration from a TOML file at `~/.notebrain/config/config.toml` (or pass `--config=/path/to/config.toml`). CLI flags always override TOML values.

Copy the template to get started:

```bash
mkdir -p ~/.notebrain/config
cp config.example.toml ~/.notebrain/config/config.toml
```

Key settings ([full reference](./config.example.toml)):

```toml
vault-path = "/path/to/Second-Brain"
vault-name = "Second-Brain"
format     = "text"              # "text", "json", "tsv"

skip-attachments = true          # ignore image/file links in graph
skip-phantom     = true          # exclude uncreated "phantom" notes
respect-exclude  = true          # honor Obsidian's ignore rules
```

### Data Location

All persistent data is stored under `~/.notebrain/`:

| Path                              | Contents                                                 |
| --------------------------------- | -------------------------------------------------------- |
| `~/.notebrain/chroma/`            | ChromaDB vector store (embeddings, metadata, link graph) |
| `~/.notebrain/config/config.toml` | User configuration file                                  |

To fully uninstall, remove the `notebrain` binary and delete `~/.notebrain/`.

## Documentation

| Guide                                                      | Description                                                |
| ---------------------------------------------------------- | ---------------------------------------------------------- |
| [Installation](wiki/Installation.md)                       | Prerequisites, pre-built binaries, building from source    |
| [Commands Reference](wiki/Commands.md)                     | Full CLI command and flag documentation                    |
| [Architecture](wiki/Architecture.md)                       | Internals: chunking pipeline, embedding, ChromaDB schema   |
| [Scheduled Ingestion](wiki/Scheduled_Ingestion.md)         | Cron and systemd timer setup for background indexing       |
| [AI Agent Skill Usage](wiki/Skill_Usage.md)                | Using the built-in AI agent skill for autonomous retrieval |
| [OpenCode Agent Integration](wiki/OpenCode_Integration.md) | Configuring NoteBrain as an OpenCode AI coding assistant   |
| [DeepWiki](https://deepwiki.com/nmdra/notebrain-cli)       | AI-generated codebase documentation                        |

## Contributing

Contributions are welcome! Please open an issue or pull request on [GitHub](https://github.com/nmdra/notebrain-cli).

This project uses [Conventional Commits](https://www.conventionalcommits.org/), Go vendoring (`vendor/`), and pre-commit hooks via [Lefthook](https://github.com/evilmartians/lefthook).

## License

[MIT License](LICENSE) — Copyright © 2026 nmdra
