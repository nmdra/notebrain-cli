# Commands Reference

NoteBrain provides a variety of commands to ingest, query, and analyze your Obsidian vault.

```text
Usage: notebrain <command> [flags]

Index and search your Obsidian vault with semantic intelligence.

NoteBrain uses local LLM embeddings to index your Markdown notes into ChromaDB,
enabling powerful semantic search, hidden graph connections, and AI-friendly
automation workflows.
```

---

## Global Flags

These flags can be applied to `notebrain` before any subcommand (e.g., `notebrain --verbose search "query"`) or defined in your configuration file.

| Flag                 | Type      | Default                           | Description                                                                                                                                |
| :------------------- | :-------- | :-------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------- |
| `--config`           | `string`  | `~/.notebrain/config/config.toml` | Path to the TOML config file.                                                                                                              |
| `--chroma-path`      | `string`  | `~/.notebrain/chroma`             | Path to ChromaDB persistent storage. Can also be set via the `$CHROMA_PATH` environment variable.                                          |
| `--vault-path`       | `string`  | _(None)_                          | **Required.** Absolute path to your Obsidian vault.                                                                                        |
| `--vault-name`       | `string`  | _(Basename of vault)_             | Obsidian vault name (used for generating `obsidian://` URI links).                                                                         |
| `--verbose`          | `boolean` | `false`                           | Enable verbose debug logging output.                                                                                                       |
| `--no-hyperlinks`    | `boolean` | `false`                           | Disable OSC 8 terminal hyperlinks in output. Can also be set via `$NO_HYPERLINKS`.                                                         |
| `--format`           | `string`  | `text`                            | Output format: `text` (standard/TUI), `json` (pretty structured JSON), `tsv` (Tab-Separated Values), or `ndjson` (Newline Delimited JSON). |
| `--jsonpath`         | `string`  | _(None)_                          | JSONPath expression to extract and filter specific fields from JSON output (e.g., `$.results[0].note_slug`).                               |
| `--include-text`     | `boolean` | `false`                           | Include matched chunk text inside structured outputs (JSON, TSV, NDJSON).                                                                  |
| `--context-window`   | `integer` | `0`                               | Fetch ±N adjacent chunks around each match for additional semantic context.                                                                |
| `--min-score`        | `float`   | `0.0`                             | Suppress search results below this similarity score (0.0 to 1.0).                                                                          |
| `--respect-exclude`  | `boolean` | `true`                            | Respect Obsidian user ignore filters and attachment folder exclusions during ingestion.                                                    |
| `--use-editor`       | `boolean` | `false`                           | Enable external editor (`$EDITOR` environment variable) integration as the default action to open notes.                                   |
| `--log-format`       | `string`  | `auto`                            | Log format: `auto` (detects TTY), `json`, or `text`.                                                                                       |
| `--log-level`        | `string`  | `info`                            | Minimum log severity to show: `info`, `debug`, `warn`, or `error`.                                                                         |
| `--skip-attachments` | `boolean` | `true`                            | Exclude attachment and image links from graph edges.                                                                                       |
| `--skip-phantom`     | `boolean` | `true`                            | Exclude uncreated notes (phantom links) from query results.                                                                                |
| `--hide-tags`        | `boolean` | `true`                            | Hide tag names (`#Tag/Subtag`) in search and graph outputs.                                                                                |
| `--version`          | `boolean` | `false`                           | Show version information.                                                                                                                  |

---

## Interactive Terminal UI (TUI)

For query-based commands (`search`, `backlinks`, `connections`, `hidden`, `boosted`, `tags`), NoteBrain launches an interactive terminal interface when output format is set to `text` and executed in a TTY:

- **Up/Down / J/K:** Navigate through results.
- **Slash (`/`):** Enter live-filter mode to filter results.
- **Enter:** Open the selected note using the default method (Obsidian, or your editor if `--use-editor` is enabled).
- **`o`:** Explicitly open the note in Obsidian.
- **`e`:** Explicitly open the note in your terminal or GUI editor (defined by the `$EDITOR` environment variable).
- **`q` / `Esc`:** Exit the TUI.

---

## Command Reference

### `ingest`

Indexes markdown files from your Obsidian vault, parses Wikilinks and tags, chunks the contents, and generates local vector embeddings.

#### Usage

```bash
notebrain ingest [<glob>] [flags]
```

#### Arguments

- `[<glob>]` _(optional)_: Glob pattern targeting specific files/folders to ingest (e.g. `Projects/**`).

#### Command-Specific Flags

| Flag                | Type      | Default | Description                                                                         |
| :------------------ | :-------- | :------ | :---------------------------------------------------------------------------------- |
| `--workers`         | `integer` | `4`     | Number of concurrent ingestion workers.                                             |
| `--min-chunk-words` | `integer` | `0`     | Skip chunks with fewer words than this (0 defaults to 10 words).                    |
| `--chunk-size`      | `integer` | `0`     | Maximum runes per chunk for the parser (0 defaults to 800 runes).                   |
| `--chunk-overlap`   | `integer` | `0`     | Overlap runes between sub-chunks when a section is split (0 defaults to 100 runes). |

#### Examples

```bash
# Ingest entire vault
notebrain ingest --vault-path "/path/to/vault"

# Ingest with customized chunk parameters and 8 worker threads
notebrain ingest --vault-path "/path/to/vault" --workers 8 --chunk-size 1000 --chunk-overlap 150

# Ingest only a specific folder pattern
notebrain ingest "Daily Notes/*.md" --vault-path "/path/to/vault"
```

---

### `search`

Performs semantic vector search across all indexed chunks in your vault. Supports filtering by sections, tags, tasks, and code.

#### Usage

```bash
notebrain search [<query>] [flags]
```

#### Arguments

- `[<query>]` _(optional)_: The semantic query string. (Can be omitted if `--interactive` or `--tag` is specified).

#### Command-Specific Flags

| Flag            | Type      | Default  | Description                                                                          |
| :-------------- | :-------- | :------- | :----------------------------------------------------------------------------------- |
| `--limit`       | `integer` | `10`     | Maximum number of results to return.                                                 |
| `--top-k`       | `integer` | `3`      | Maximum number of chunks to return per note.                                         |
| `--section`     | `string`  | _(None)_ | Filter results by heading path.                                                      |
| `--tag`         | `string`  | _(None)_ | Filter results by tag name (prefixed `#` is optional).                               |
| `--has-tasks`   | `boolean` | `false`  | Only return chunks containing markdown task lists (`- [ ]`).                         |
| `--has-code`    | `boolean` | `false`  | Only return chunks containing code blocks.                                           |
| `--interactive` | `boolean` | `false`  | Launch a live interactive search TUI where you can type queries and preview results. |
| `--split`       | `boolean` | `false`  | Split query string by delimiters (comma, pipe, semicolon) or execute multi-positional queries. |
| `--split-by`    | `string`  | `,|;`    | Delimiters used to split query strings when `--split` is active.                     |

#### Examples

```bash
# Basic semantic search
notebrain search "reconciliation loop in kubernetes" --limit 5

# Search specifically for tasks under the Kubernetes tag
notebrain search "deploy service" --tag "Kubernetes" --has-tasks

# Multi-query search (positional arguments)
notebrain search "message brokers" "redis queue"

# Multi-query search using delimiter splitting
notebrain search "redis, streams, pubsub" --split

# Multi-query search with tags hidden in output
notebrain search "redis, streams" --split --hide-tags

# Launch live-search interactive terminal
notebrain search --interactive
```

#### How Multi-Query Matching & Ranking Works
When multiple queries are provided (either via multiple positional arguments or via `--split`):
1. **Semantic Vector Matching**: NoteBrain embeds each query independently into a 384-dimensional vector using `MiniLM-L6-v2`. Matching is based **100% on semantic vector similarity** (cosine distance in ChromaDB), not exact keyword or substring matching. A note can match a query even if it uses completely different terminology or synonyms.
2. **Multi-Hit Boosting**: When a note chunk is semantically relevant to multiple queries in your search, NoteBrain boosts its rank! Results are sorted using a two-tier strategy:
   - **Primary Sort**: Descending order by the number of matched query topics (`len(MatchedQueries)`). Chunks bridging multiple concepts (e.g., matching both `"message brokers"` and `"redis queue"`) appear at the top.
   - **Secondary Sort**: Descending order by maximum cosine similarity score within each match-count tier.
3. **Hit Attribution**: In terminal text mode, multi-hit chunks display attribution tags indicating which query vectors retrieved them (e.g., `[hits: "message brokers", "redis queue"]`). In structured JSON outputs, each item includes a `matched_queries` array.

---

### `get`

Reconstructs and displays the complete markdown text of a note by joining all of its indexed chunks.

#### Usage

```bash
notebrain get <slug> [flags]
```

#### Arguments

- `<slug>` _(required)_: Note slug (e.g. `kubernetes-native-applications`) or vault file path.

#### Examples

```bash
# Retrieve full content of a note by slug
notebrain get "kubernetes-native-applications"

# Retrieve full content of a note, outputting to JSON
notebrain get "kubernetes-native-applications" --format json
```

---

### `backlinks`

Finds all notes linking to the target note using the local Wikilink graph.

#### Usage

```bash
notebrain backlinks <note> [flags]
```

#### Arguments

- `<note>` _(required)_: The target note slug or title.

#### Examples

```bash
# Find what notes link to "Redis"
notebrain backlinks "Redis"
```

---

### `connections`

Performs breadth-first traversal of the Wikilink graph to find connected notes up to a specified number of hops.

#### Usage

```bash
notebrain connections <note> [flags]
```

#### Arguments

- `<note>` _(required)_: The starting note slug or title.

#### Command-Specific Flags

| Flag     | Type      | Default | Description                               |
| :------- | :-------- | :------ | :---------------------------------------- |
| `--hops` | `integer` | `2`     | Maximum number of graph hops to traverse. |

#### Examples

```bash
# Find notes connected within 2 hops of "Redis"
notebrain connections "Redis" --hops 2
```

---

### `hidden`

Discovers "hidden" semantic connections: notes that are semantically similar but do not have direct Wikilinks in Obsidian.

#### Usage

```bash
notebrain hidden <note> [flags]
```

#### Arguments

- `<note>` _(required)_: The target note slug or title.

#### Command-Specific Flags

| Flag      | Type      | Default | Description                                     |
| :-------- | :-------- | :------ | :---------------------------------------------- |
| `--limit` | `integer` | `10`    | Maximum number of hidden connections to return. |

#### Examples

```bash
# Discover 5 closest semantic notes to "Redis" that are not linked
notebrain hidden "Redis" --limit 5
```

---

### `tags`

Finds notes sharing tags with a given note, ranked by the number of shared tags.

#### Usage

```bash
notebrain tags <note> [flags]
```

#### Arguments

- `<note>` _(required)_: The target note slug or title.

#### Command-Specific Flags

| Flag           | Type      | Default | Description                                        |
| :------------- | :-------- | :------ | :------------------------------------------------- |
| `--min-shared` | `integer` | `1`     | Minimum number of shared tags to include a result. |

#### Examples

```bash
# Find notes sharing at least 2 tags with "Redis"
notebrain tags "Redis" --min-shared 2
```

---

### `boosted`

Graph-boosted semantic search. Combines semantic vector similarity with Wikilink graph distance from a seed note, boosting similarity scores for notes structurally connected to the seed.

#### Usage

```bash
notebrain boosted <query> --seed=STRING [flags]
```

#### Arguments

- `<query>` _(required)_: Search query.

#### Command-Specific Flags

| Flag      | Type      | Default  | Description                                                     |
| :-------- | :-------- | :------- | :-------------------------------------------------------------- |
| `--seed`  | `string`  | _(None)_ | **Required.** The origin note slug or title for graph boosting. |
| `--boost` | `float`   | `1.5`    | Multiplier applied to scores of graph-connected results.        |
| `--limit` | `integer` | `10`     | Maximum number of results to return.                            |

#### Examples

```bash
# Perform search boosted by structural connections to "Redis"
notebrain boosted "caching strategies" --seed "Redis" --boost 2.0 --limit 5
```

---

### `stats`

Displays statistics for your NoteBrain collection (total number of indexed chunks and links).

#### Usage

```bash
notebrain stats [flags]
```

#### Examples

```bash
notebrain stats
```

---

### `reset`

Drops all NoteBrain collections (`nb_chunks` and `nb_links`) and starts fresh. This operation is irreversible.

#### Usage

```bash
notebrain reset [flags]
```

_Note: For automated scripts, you can bypass the interactive confirmation prompt by piping `yes`:_

```bash
echo yes | notebrain reset
```

---

### `version`

Prints version information, including build commit hash and compile date.

#### Usage

```bash
notebrain version [flags]
```

---

## ⚙️ Configuration File

Any global flag can be persistently configured in `~/.notebrain/config/config.toml` (or custom path passed to `--config`). Keys in the config file support interchangeable `kebab-case` and `snake_case` styles.

```toml
# ~/.notebrain/config/config.toml
vault-path = "/home/user/Obsidian/MainVault"
vault-name = "MainVault"
chroma-path = "~/.notebrain/chroma"
format = "json"
verbose = true
context-window = 1
skip-attachments = true
```

---

## Machine-Readable Output & AI Chain Automation

Using output formats like JSON or NDJSON and extracting fields via `--jsonpath` allows easy piping to shell tools and AI agents:

```bash
# Extract the slug of the top result
TOP_SLUG=$(notebrain search "golang channels" --limit 1 --jsonpath="$.results[0].note_slug")

# Pass it to fetch full content
notebrain get "$TOP_SLUG" --jsonpath="$.text"
```
