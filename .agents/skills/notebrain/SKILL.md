---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: MIT
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain is a high-performance Go CLI tool that indexes an Obsidian vault into a local ChromaDB vector database. It provides semantic search, graph traversal, backlink discovery, hidden relationship finding, full note retrieval, and tag analysis across markdown notes.

## Core Execution Principles & Rationale

To operate efficiently and prevent wasted tokens or hung sessions, follow these foundational principles:

1. **NoteBrain Only — No Generic File Search**: Never use `grep`, `find`, `ls` or ad-hoc shell scripting against the vault's markdown files to answer the user's question. Treat `notebrain` as the only interface to the vault's content. If a query returns nothing useful, refine the `notebrain` query rather than falling back to a filesystem search.
2. **Non-Interactive Execution (`--format json`)**: Always specify `--format json` (or `ndjson`/`tsv`) on query commands so you receive structured, parseable data immediately without launching the interactive TUI. All examples below assume `--format json` unless noted.
3. **AI Agent Command Chaining (`--jsonpath`)**: Whenever you only need specific fields (like extracting note slugs or text to pipe into follow-up commands), use `--jsonpath` (e.g., `--jsonpath="$.results[0].note_slug"`). Scalar values output as raw strings and arrays print each item on a new line, avoiding `jq`.
4. **Retrieve Complete Notes (`notebrain get`)**: When `search` returns a relevant note chunk, use `notebrain get <note-slug-or-path>` rather than guessing chunk indices to retrieve the complete reconstructed markdown note.
5. **Retrieve Content for Synthesis (`--include-text`)**: By default, query commands return lightweight metadata envelopes. Whenever your task requires summarizing or reasoning about chunk content directly from search results, append `--include-text`.
6. **CLI Syntax Rules**: In development environments, execute `./notebrain` if `notebrain` is not in PATH. Always encapsulate note titles, tags, and queries in double quotes. Strictly use `--vault-path` and `--chroma-path` (never `--vault` or `--db`). For graph and note commands (`backlinks`, `connections`, `hidden`, `tags`, `get`), pass exactly one positional argument: `<note>`. For `boosted` search, specify `--seed=<slug>`. For resets, pipe confirmation (`echo yes | notebrain reset`).
7. **Graph & Link Filtering (`--skip-attachments`, `--skip-phantom`)**: By default, NoteBrain excludes image/attachment links (`.webp`, `.png`, `.pdf`, `.canvas`) from graph edges (`--skip-attachments=true`), and excludes uncreated "phantom" notes (wikilinks without a `.md` file on disk) from results (`--skip-phantom=true`). To explore missing notes or broken links, pass `--skip-phantom=false` (marked with `"is_phantom": true` in JSON or `[phantom]` in text).
8. **Intelligent Query Splitting (`--split`)**: When researching compound questions, long queries, or orthogonal topics (e.g., comparing two technologies or looking for intersections between domains), do not issue multiple separate search commands. Instead, intelligently split the query into distinct conceptual terms using positional arguments (`notebrain search "redis pubsub" "kafka message brokers"`) or delimiter splitting (`notebrain search "redis pubsub, kafka message brokers" --split`). **Why?** NoteBrain uses multi-hit score boosting: chunks that match multiple query topics are automatically ranked above single-topic matches, instantly surfacing synthesizing notes that bridge those domains. For simple, single-concept lookups, keep the query intact without splitting.

---

## Token Budget Defaults & Session Caching

To avoid burning unnecessary tokens and latency:

- **Limit Result Sizes**: Default `--limit` to 5 (not the tool's default 10) unless the user asks for broad coverage.
- **Selective `--include-text`**: Only pass `--include-text` on the _first_ command in a retrieval workflow. For follow-up commands (`connections`, `tags`, `backlinks` used purely to map structure), omit `--include-text` — slugs, titles, and scores are enough to decide what (if anything) to `get` in full.
- **Prefer `--jsonpath`**: Extract only the fields you will actually use rather than loading full JSON envelopes into context.
- **Reuse Within a Session**: If a `backlinks`, `connections`, or `hidden` call was already run for a given seed slug earlier in this conversation, reuse those results instead of re-querying, unless the user asked to re-ingest or the vault may have changed.

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
# Basic single-topic semantic search
notebrain search "kubernetes reconciliation" --tag="Kubernetes" --limit 5 --include-text

# Multi-topic search across orthogonal concepts (surfaces bridging notes via multi-hit boosting)
notebrain search "message brokers" "redis queue" --limit 5 --include-text

# Delimiter-based query splitting for compound research questions
notebrain search "redis, streams, pubsub" --split --limit 5 --include-text
```

#### Key search flags

| Flag                 | Purpose                                                                                                                                            | Default |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--top-k N`          | Maximum chunks to retain **per note**. Prevents one long note from dominating results while preserving depth.                                      | `3`     |
| `--context-window N` | Fetches ±N adjacent chunks around each match into `context`. Use for lightweight multi-result context; use `get` only when you need the full note. | `0`     |
| `--split`            | Split query string by delimiters (comma, pipe, semicolon) or execute multi-positional queries. Highly recommended for multi-topic research queries. | off     |
| `--split-by "CHARS"` | Delimiter characters used to tokenize query strings when `--split` is active.                                                                      | `",|;"`  |
| `--has-tasks`        | Only return chunks that contain task lists (checkboxes).                                                                                           | off     |
| `--has-code`         | Only return chunks that contain fenced code blocks.                                                                                                | off     |
| `--section`          | Filter results to chunks under a specific heading path (e.g., `"Architecture > Components"`).                                                      | —       |
| `--limit N`          | Maximum total results to return.                                                                                                                   | `10`    |
| `--tag "TagName"`    | Filter or search by tag name.                                                                                                                      | —       |
| `--min-score F`      | Suppress results below this similarity score (0–1).                                                                                                | `0.4`   |
| `--hide-tags`        | Hide tag names (`#Tag/Subtag`) in search and graph outputs.                                                                                        | `true`  |

### Complete Note Retrieval (`get`)

```bash
notebrain get "02areaskubernetesckadkubernetes-native-applications"
```

### Graph Connections, Hidden Links & Tags (`connections`, `hidden`, `tags`, `backlinks`)

All graph and tag commands accept **exactly one positional argument**: the target note slug.

```bash
# Find explicit backlinks pointing to a note
notebrain backlinks "kubernetes-architecture" --include-text

# Traverse wikilink graph connections up to N hops
notebrain connections "kubernetes-architecture" --hops 2

# Find notes sharing common tags with the target note
notebrain tags "kubernetes-architecture" --min-shared 1

# Discover unlinked but semantically similar notes
notebrain hidden "kubernetes-architecture" --limit 5 --include-text
```

#### Graph & Link Filtering Flags

| Flag                 | Purpose                                                                                               | Default |
| -------------------- | ----------------------------------------------------------------------------------------------------- | ------- |
| `--skip-attachments` | Exclude attachment and image links (e.g., `.webp`, `.png`, `.canvas`) from graph edges and backlinks. | `true`  |
| `--skip-phantom`     | Exclude uncreated notes (phantom wikilinks without a markdown file on disk) from results.             | `true`  |

### Graph-Boosted Search (`boosted`)

```bash
notebrain boosted --seed="kubernetes-architecture" "control plane components" --boost 1.5
```

---

## JSON Output Schema

Every query command wraps results in a JSON envelope. Understanding the field specification is essential for reliable extraction:

| Field          | Present When                    | Description                                                                                |
| -------------- | ------------------------------- | ------------------------------------------------------------------------------------------ |
| `note_slug`    | Always                          | URL-safe identifier derived from the file path.                                            |
| `title`        | Always                          | Note title from frontmatter or filename.                                                   |
| `file_path`    | Always                          | Relative path within the vault.                                                            |
| `score`        | Always                          | Similarity score (0–1) for search; hop count for connections.                              |
| `chunk_index`  | Search, hidden, boosted         | Which chunk of the note matched (0-indexed).                                               |
| `tags`         | When note has tags              | Array of tag strings.                                                                      |
| `heading_path` | When chunk is under a heading   | Breadcrumb path like `"Section > Subsection"`.                                             |
| `text`         | When `--include-text` is passed | The matched chunk's full markdown text, with code blocks preserved.                        |
| `context`      | When `--context-window N` > 0   | Array of ±N adjacent chunk texts, ordered by chunk index.                                  |
| `extra`           | Connections, tags, boosted      | Command-specific info (e.g., `"2 hop(s)"`, `"graph-boosted"`).                             |
| `is_phantom`      | When `--skip-phantom=false`     | Boolean (`true`) if the note is an uncreated phantom link without a markdown file on disk. |
| `matched_queries` | When results match queries      | Array of query strings that matched this chunk (multi-hit attribution).                    |

---

## Retrieval Workflow (Tiered, Not Automatic)

To prevent excessive tool calls and context bloat, follow a progressive, conditional retrieval strategy:

1. **Start Lean**: Always start with a lean search: `notebrain search "topic" --context-window 1 --top-k 2 --include-text --limit 5`. For compound or multi-topic questions, intelligently split into distinct concepts: `notebrain search "topic A" "topic B" --context-window 1 --top-k 2 --include-text --limit 5`.
2. **Check Score & Sufficiency**: Check the top result's `score`. If `score ≥ 0.75` and the returned context fully answers the question, **stop here** — do not run further commands.
3. **Escalate Conditionally**: Only escalate if the question or initial findings require it:
   - Ask is about **connections/related notes** → also run `connections --hops 2` (no `--include-text`; slugs/titles are enough for a graph map).
   - Ask is about **what links here** → run `backlinks` instead of the full chain.
   - Ask is **exploratory** ("what do I know about X") and step 1 was thin or low-score (`score < 0.75`) → run `hidden` too.
4. **Avoid Blanket Chaining**: Never run all four commands (`search → backlinks → connections → hidden`) unless the user explicitly asks for a full comprehensive map of the topic across the entire vault.

### Targeted Retrieval Patterns

```bash
# Extract top slug directly via JSONPath
SLUG=$(notebrain search "message broker backpressure" --limit 3 --top-k 2 \
  --context-window 1 --include-text --jsonpath="$.results[0].note_slug")

# Fetch complete note text for a specific hit
notebrain get "$SLUG" --jsonpath="$.text"

# Find code examples about a topic
notebrain search "docker compose networking" --has-code --include-text

# Find actionable tasks related to a project
notebrain search "sprint planning" --has-tasks --include-text
```

---

## Configuration Hierarchy

NoteBrain resolves settings in priority order:

1. CLI command flags (`--vault-path`, `--vault-name`, `--chroma-path`, `--top-k`, `--context-window`)
2. Configuration file (`~/.notebrain/config/config.toml` or specified via `--config`)
