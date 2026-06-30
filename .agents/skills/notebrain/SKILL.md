---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
---

# NoteBrain CLI Skill for AI Agents

NoteBrain is a high-performance Go CLI tool that indexes an Obsidian vault into a local ChromaDB vector database. It provides semantic search, graph traversal, backlink discovery, hidden relationship finding, full note retrieval, and tag analysis across markdown notes.

## Core Execution Principles & Rationale

To operate efficiently and prevent wasted tokens or hung sessions, follow these foundational principles:

1. **Non-Interactive Execution (`--format json`)**: By default, NoteBrain launches an interactive terminal TUI (Bubble Tea) designed for human browsing. This interactive interface will hang automated agent sessions. Always specify `--format json` (or `--format ndjson` / `--format tsv`) on query commands so you receive structured, parseable data immediately. Note that all JSON envelope fields use clean `snake_case` keys (`note_slug`, `title`, `file_path`, `score`, `tags`, `text`).
2. **AI Agent Command Chaining (`--jsonpath`)**: Whenever you only need specific fields (like extracting note slugs or text to pipe into follow-up commands), use `--jsonpath` (e.g., `--jsonpath="$.results[0].note_slug"` or `--jsonpath="$.results[*].note_slug"`). When `--jsonpath` is used, scalar values are output as raw unquoted strings and arrays print each item on a new line, enabling seamless shell variable extraction without needing `jq`.
3. **Retrieve Complete Notes (`notebrain get`)**: When `search` returns a relevant note chunk, use `notebrain get <note-slug-or-path>` rather than guessing chunk indices to retrieve the complete, reconstructed markdown note text stitched together across all its indexed chunks.
4. **Retrieve Content for Synthesis (`--include-text`)**: By default, `search` and traversal commands return lightweight metadata envelopes (titles, file paths, tags, similarity scores). Whenever your task requires summarizing or reasoning about individual chunks directly from search results, append `--include-text`.
5. **Binary Resolution & Quoting**: In development environments, execute `./notebrain` if `notebrain` is not in system PATH. Note titles often contain spaces or brackets (`"Q3 Planning [Draft]"`), so always encapsulate note titles, tags, and search queries within double quotes.

---

## Command Selection Guide

Select the specialized command tailored to the user's analytical goal:

| User Intent                                         | Command       | Why & How to Use                                                                                                                |
| --------------------------------------------------- | ------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| "What do my notes say about X?"                     | `search`      | Performs vector similarity search across all note chunks. Use `--tag="TagName"` to filter by tag.                               |
| "Read the full content of note Y"                   | `get`         | Retrieves and reconstructs the complete note text and metadata by slug or file path.                                            |
| "What links directly to this note?"                 | `backlinks`   | Finds explicit `[[wikilink]]` references pointing to the target note.                                                           |
| "What is structurally nearby in the graph?"         | `connections` | Executes breadth-first search (BFS) over wikilinks up to `--hops N`. Keep `--hops 1` or `--hops 2` to avoid exponential blowup. |
| "What is related in meaning but NOT linked?"        | `hidden`      | Surfaces unlinked-but-semantically-similar notes. Highly valuable for discovering conceptual bridges.                           |
| "Find concepts related to X centered around note Y" | `boosted`     | Combines vector similarity with graph proximity to a `--seed` note.                                                             |
| "What notes share tags with X?"                     | `tags`        | Analyzes tag overlap. Returns clean tag strings in the `tags` array.                                                            |
| "Is the database up to date?"                       | `stats`       | Outputs collection counts (`chunks`, `links`). Supports `--format=json` and `--jsonpath`.                                       |
| "Index or re-index the vault"                       | `ingest`      | Synchronizes markdown notes into ChromaDB. Re-ingestion is idempotent.                                                          |

---

## Command Syntax

### Semantic Search & Tag Filtering (`search`)

```bash
notebrain search "kubernetes reconciliation" --tag="Kubernetes" --limit 5 --format json --include-text
```

### Complete Note Retrieval (`get`)

```bash
notebrain get "02areaskubernetesckadkubernetes-native-applications" --format json
```

### Graph Connections & Hidden Links (`connections`, `hidden`)

```bash
notebrain connections "Distributed Systems" --hops 2 --format json
notebrain hidden "Microservices" --limit 5 --format json --include-text
```

### AI Agent Chaining Pipeline (Search -> Get Full Note -> Backlinks)

```bash
# 1. Extract the top matching note slug cleanly into a shell variable
SLUG=$(notebrain search "message broker backpressure" --limit 1 --jsonpath="$.results[0].note_slug")

# 2. Fetch the complete reconstructed note text
notebrain get "$SLUG" --jsonpath="$.text"

# 3. Find all backlink note slugs pointing to this note
notebrain backlinks "$SLUG" --jsonpath="$.results[*].note_slug"
```

---

## Configuration Hierarchy

NoteBrain resolves settings in priority order:

1. CLI command flags (`--vault-path`, `--chroma-path`)
2. Environment variables defined in local `.env` (`OBSIDIAN_VAULT_PATH`, `OBSIDIAN_VAULT_NAME`)
3. Global configuration file (`~/.notebrain/config/config.toml`)
