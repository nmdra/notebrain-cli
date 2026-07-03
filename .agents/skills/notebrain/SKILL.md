---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: Apache-2.0
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain is a high-performance Go CLI tool that indexes an Obsidian vault into a local ChromaDB vector database. It provides semantic search, graph traversal, backlink discovery, hidden relationship finding, full note retrieval, and tag analysis across markdown notes.

## Core Execution Principles & Rationale

To operate efficiently and prevent wasted tokens or hung sessions, follow these foundational principles:

1. **NoteBrain Only — No Generic File Search**: Never use `grep`, `find`, `ls` or ad-hoc shell scripting against the vault's markdown files to answer the user's question. NoteBrain's commands are purpose-built on top of the indexed vector and graph database and will always produce higher-quality, more relevant results than a filesystem-level text search. Treat `notebrain` as the only interface to the vault's content. If a `notebrain` command appears to return nothing useful, refine the `notebrain` query (different phrasing, broader `--limit`, alternate command) rather than falling back to a generic file search.
2. **Non-Interactive Execution (`--format json`)**: By default, NoteBrain launches an interactive terminal TUI (Bubble Tea) designed for human browsing. This interactive interface will hang automated agent sessions. Always specify `--format json` (or `--format ndjson` / `--format tsv`) on query commands so you receive structured, parseable data immediately. Note that all JSON envelope fields use clean `snake_case` keys (`note_slug`, `title`, `file_path`, `score`, `tags`, `text`).
3. **AI Agent Command Chaining (`--jsonpath`)**: Whenever you only need specific fields (like extracting note slugs or text to pipe into follow-up commands), use `--jsonpath` (e.g., `--jsonpath="$.results[0].note_slug"` or `--jsonpath="$.results[*].note_slug"`). When `--jsonpath` is used, scalar values are output as raw unquoted strings and arrays print each item on a new line, enabling seamless shell variable extraction without needing `jq`.
4. **Retrieve Complete Notes (`notebrain get`)**: When `search` returns a relevant note chunk, use `notebrain get <note-slug-or-path>` rather than guessing chunk indices to retrieve the complete, reconstructed markdown note text stitched together across all its indexed chunks. Never reach for `cat` on the underlying vault file — `get` reconstructs chunked notes correctly and respects the indexed state of the vault.
5. **Retrieve Content for Synthesis (`--include-text`)**: By default, `search` and traversal commands return lightweight metadata envelopes (titles, file paths, tags, similarity scores). Whenever your task requires summarizing or reasoning about individual chunks directly from search results, append `--include-text`.
6. **Binary Resolution & Quoting**: In development environments, execute `./notebrain` if `notebrain` is not in system PATH. Note titles often contain spaces or brackets (`"Q3 Planning [Draft]"`), so always encapsulate note titles, tags, and search queries within double quotes.

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

---

## Deepening Answer Quality: Backlinks + Connections First

Before answering any "what do I know about X" or "summarize my notes on X" style question, don't stop at a single `search` call. Search results alone only surface chunks that are semantically close to the query — they miss the surrounding context the user has deliberately built into their graph. Always widen the picture using `backlinks`, `connections` or `boosted`:

1. **Search to find the anchor note(s)** — run `search` to identify the one or two most relevant notes (the "seed" notes) for the topic.
2. **Pull backlinks for each seed** — run `notebrain backlinks <seed-slug> --format json --include-text` to extract every note that explicitly links into the seed. These are notes the user has manually curated as related, so their content is almost always high-signal and should be weighted heavily in your synthesis.
3. **Walk connections outward** — run `notebrain connections <seed-slug> --hops 2 --format json` to map the local graph neighborhood around the seed. This reveals structurally adjacent notes (e.g., notes two links away) that may not show up in a pure vector search but are part of the same knowledge cluster.
4. **Check for hidden links** — run `notebrain hidden <seed-slug> --include-text` to catch conceptually related notes the user hasn't linked yet. Call these out explicitly to the user as potential missing links in their vault, since this is one of NoteBrain's most valuable differentiators over plain search.
5. **Synthesize, don't just list** — combine the seed note(s), their backlinks, their `connections` neighborhood, and any `hidden` results into a single coherent answer, distinguishing what's explicitly linked (high confidence) from what's only semantically similar (worth double-checking).

This `search → backlinks → connections → hidden` chain should be the default workflow for any non-trivial exploratory question, not just a single `search` call, because it surfaces both the explicit structure the user built and the implicit structure NoteBrain can detect.

### AI Agent Chaining Pipeline (Search → Get Full Note → Backlinks → Connections)

```bash
# 1. Extract the top matching note slug cleanly into a shell variable
SLUG=$(notebrain search "message broker backpressure" --limit 1 --jsonpath="$.results[0].note_slug")

# 2. Fetch the complete reconstructed note text
notebrain get "$SLUG" --jsonpath="$.text"

# 3. Find all backlink note slugs pointing to this note, with their content,
#    to ground the answer in what the user has explicitly linked
notebrain backlinks "$SLUG" --format json --include-text

# 4. Walk the graph neighborhood to surface structurally adjacent notes
#    that vector search alone would miss
notebrain connections "$SLUG" --hops 2 --format json

# 5. Surface unlinked-but-related notes as potential missing connections
notebrain hidden "$SLUG" --limit 5 --format json --include-text
```

---

## Configuration Hierarchy

NoteBrain resolves settings in priority order:

1. CLI command flags (`--vault-path`, `--chroma-path`)
2. Environment variables defined in local `.env` (`OBSIDIAN_VAULT_PATH`, `OBSIDIAN_VAULT_NAME`)
3. Global configuration file (`~/.notebrain/config/config.toml`)

---

## License

This skill is distributed under the Apache License, Version 2.0. See `LICENSE.txt` for the full license text.

## Allowed Tools

This skill is restricted to invoking the `notebrain` CLI binary (and its local `./notebrain` form) via the Bash tool. No other shell commands — including generic file-search utilities like `grep`, `find` or `ls` against vault files — are sanctioned for use within this skill's workflows.
