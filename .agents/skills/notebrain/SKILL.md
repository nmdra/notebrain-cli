---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
---

# NoteBrain CLI Skill for AI Agents

NoteBrain is a high-performance Go CLI tool that indexes an Obsidian vault into a local ChromaDB vector database. It provides semantic search, graph traversal, backlink discovery, hidden relationship finding, and tag analysis across markdown notes.

## Core Execution Principles & Rationale

To operate efficiently and prevent wasted tokens or hung sessions, follow these foundational principles:

1. **Non-Interactive Execution (`--format json`)**: By default, NoteBrain launches an interactive terminal TUI (Bubble Tea) designed for human browsing. This interactive interface will hang automated agent sessions. Always specify `--format json` (or `--format ndjson` / `--format tsv`) on every query command so you receive structured, parseable data immediately.
2. **Retrieve Content for Synthesis (`--include-text`)**: By default, query commands return lightweight metadata envelopes (titles, file paths, similarity scores) to conserve bandwidth. Whenever your task requires summarizing, analyzing, or reasoning about the actual contents of notes, always append `--include-text` so the markdown chunk text is returned within the JSON envelope.
3. **Binary Resolution**: In development environments, the compiled executable often resides directly in the repository root rather than system PATH. Check `which notebrain` first; if unavailable, execute `./notebrain` from the workspace root.
4. **Exact Quoting**: Note titles in Obsidian frequently contain spaces, brackets, or punctuation (`"Q3 Planning [Draft]"`). Always encapsulate note titles and search queries within double quotes to prevent shell parsing errors.
5. **Parse JSON Envelopes Correctly**: Inspect the parsed JSON response before extracting results. Check for a top-level `"error"` key before accessing `"results"`. Note that an empty `"results": []` array with `"total": 0` represents a successful query with zero matches, not an execution failure.

---

## Command Selection Guide

Select the specialized command tailored to the user's analytical goal:

| User Intent | Command | Why & How to Use |
|---|---|---|
| "What do my notes say about X?" | `search` | Performs vector similarity search across all note chunks. Ideal entry point for broad topical inquiries. |
| "What links directly to this note?" | `backlinks` | Finds explicit `[[wikilink]]` references pointing to the target note. |
| "What is structurally nearby in the graph?" | `connections` | Executes breadth-first search (BFS) over wikilinks up to `--hops N`. Use when analyzing structural network topology rather than pure semantic similarity. Keep `--hops 1` or `--hops 2` to avoid exponential blowup. |
| "What is related in meaning but NOT linked?" | `hidden` | Surfaces unlinked-but-semantically-similar notes. Highly valuable for discovering forgotten connections or conceptual bridges in knowledge bases. |
| "Find concepts related to X centered around note Y" | `boosted` | Combines vector similarity with graph proximity to a `--seed` note. Multiplies scores for notes close to the seed in the wikilink graph. |
| "What notes share tags with X?" | `tags` | Analyzes tag overlap. Use `--min-shared` to filter by threshold. |
| "Is the database up to date?" | `stats` | Outputs quick collection counts. Run if you suspect vault modifications have occurred. |
| "Index or re-index the vault" | `ingest` | Synchronizes markdown notes into ChromaDB. Re-ingestion is idempotent; run only when `stats` indicates drift or missing notes. |

---

## Command Syntax & Examples

### Semantic Search (`search`)
```bash
notebrain search "how do message brokers handle backpressure?" --limit 5 --format json --include-text
```
*Optional filters:* `--section="Architecture"` (filter by markdown heading), `--has-code` (only chunks with code blocks), `--min-score=0.5` (similarity threshold).

### Graph Connections (`connections`)
```bash
notebrain connections "Distributed Systems" --hops 2 --format json
```

### Hidden Relationships (`hidden`)
```bash
notebrain hidden "Microservices" --limit 5 --format json --include-text
```

### Graph-Boosted Search (`boosted`)
```bash
notebrain boosted "caching strategies" --seed "Redis" --boost 2.0 --limit 5 --format json --include-text
```

---

## Configuration Hierarchy

NoteBrain resolves settings in priority order:
1. CLI command flags (`--vault-path`, `--chroma-path`)
2. Environment variables defined in local `.env` (`OBSIDIAN_VAULT_PATH`, `OBSIDIAN_VAULT_NAME`)
3. Global configuration file (`~/.notebrain/config/config.toml`)

If a command fails due to a missing vault path, check `.env` or `config.toml` before asking the user for manual path input.
