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

| Flag                | Type      | Default                           | Description                                                                                                  |
| :------------------ | :-------- | :-------------------------------- | :----------------------------------------------------------------------------------------------------------- |
| `--config`          | `string`  | `~/.notebrain/config/config.toml` | Path to the TOML config file.                                                                                |
| `--chroma-path`     | `string`  | `~/.notebrain/chroma`             | Path to ChromaDB persistent storage. Can also be set via the `$CHROMA_PATH` environment variable.            |
| `--vault-path`      | `string`  | _(None)_                          | **Required.** Absolute path to your Obsidian vault.                                                          |
| `--vault-name`      | `string`  | _(Basename of vault)_             | Obsidian vault name (used for generating `obsidian://` URI links).                                           |
| `--verbose`         | `boolean` | `false`                           | Enable verbose debug logging output.                                                                         |
| `--no-hyperlinks`   | `boolean` | `false`                           | Disable OSC 8 terminal hyperlinks in output. Can also be set via `$NO_HYPERLINKS`.                           |
| `--format`          | `string`  | `text`                            | Output format: `text` (standard text), `json` (pretty structured JSON), or `tsv` (Tab-Separated Values).     |
| `--jsonpath`        | `string`  | _(None)_                          | JSONPath expression to extract and filter specific fields from JSON output (e.g., `$.results[0].note_slug`). |
| `--include-text`    | `boolean` | `false`                           | Include matched chunk text inside structured outputs (JSON, TSV).                                            |
| `--context-window`  | `integer` | `0`                               | Fetch ±N adjacent chunks around each match for additional semantic context.                                  |
| `--min-score`       | `float`   | `0.0`                             | Suppress search results below this similarity score (0.0 to 1.0).                                            |
| `--respect-exclude` | `boolean` | `true`                            | Respect Obsidian user ignore filters and attachment folder exclusions during ingestion.                      |
| `--log-format`      | `string`  | `auto`                            | Log format: `auto` (detects TTY), `json`, or `text`.                                                         |
| `--log-level`       | `string`  | `info`                            | Minimum log severity to show: `info`, `debug`, `warn`, or `error`.                                           |
| `--hide-tags`       | `boolean` | `true`                            | Hide tag names (`#Tag/Subtag`) in search and graph outputs.                                                  |
| `--show-file-path`  | `boolean` | `true`                            | Include `file_path` in outputs (`--show-file-path=false` to omit).                                           |

### Token Efficiency & Quiet Mode for AI Agents

When executing NoteBrain queries inside AI agent workflows, automated pipelines, or background scripts, controlling token footprint and suppressing interactive formatting is essential:

1. **Automatic Quiet Mode (`--quiet`)**:
   Whenever a non-interactive machine format (`--format=json`, `tsv`, or `--jsonpath`) is specified, NoteBrain automatically activates quiet mode (`embedder.WithQuiet`). This suppresses background log output, ensuring stdout is 100% clean and uncorrupted for JSON parsers and AI agents.
2. **Compact JSON Envelopes**:
   By default, JSON output includes essential properties cleanly formatted. You can use `--show-file-path=false` to strip file paths and reduce token consumption for Large Language Models. Similarity scores (`score`) are rounded cleanly to 4 decimal places (`0.8520`), and query headers (`query`) are stripped of terminal decorations.
3. **Non-Redundant Context Windows (`--context-window N`)**:
   When `--context-window N` (e.g., `--context-window 1` or `2`) is passed alongside `--include-text`, NoteBrain fetches $\pm N$ adjacent chunks into the `context` array while specifically excluding the matched chunk (`text`) itself from the array (`PopulateContext`), eliminating duplicated text across `text` and `context`.
   | `--version` | `boolean` | `false` | Show version information. |

---

### Context-Aware Empty Result Guidance

When a search or graph command returns zero results in standard terminal `text` format (`--format=text`), NoteBrain displays actionable, tailored tips formatted in italicized amber (`hintStyle`) under the command header instead of generic `(no results)` text:

- **`backlinks`**: Suggests verifying whether other notes link to the target or re-indexing via `notebrain ingest`.
- **`connections`**: Suggests increasing `--hops` or checking for valid Wikilinks.
- **`hidden`**: Suggests trying `--include-linked` to include notes that may already be linked, or re-indexing if the note is too unique.
- **`tags`**: Suggests checking note tags or lowering `--min-shared`.
- **`search` / `boosted`**: Suggests broadening search terms, adjusting `--boost`, or running `notebrain ingest`.

> _Note: To ensure compatibility with automated scripts and AI agents, contextual hints only appear in standard `text` output and are strictly omitted from machine formats (`json`, `tsv`, `--jsonpath`)._

---

## Note Resolution (`<note>` argument)

Commands targeting a specific note (`backlinks`, `connections`, `hidden`, `tags`, `boosted --seed=<note>`, `get`) accept any of the following formats for the `<note>` parameter:

1. **Exact Note Slug**: The normalized stored identifier (e.g., `00fleeting-noteskubernetes-networking-toolsspiffe`).
2. **Note Title**: Case-insensitive note title (e.g., `"SPIFFE"` or `"Rust Programming"`).
3. **Filename**: Exact case-insensitive file name (e.g., `"SPIFFE.md"`).
4. **Partial Path / Suffix**: End of the relative path inside your vault (e.g., `"tools/SPIFFE.md"`).

If multiple notes in your vault share the exact same title or filename across different directories, NoteBrain will return an ambiguity error listing the exact candidate slugs so you can specify the exact path or slug.

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

| Flag            | Type      | Default  | Description                                                                                    |
| :-------------- | :-------- | :------- | :--------------------------------------------------------------------------------------------- |
| `--limit`       | `integer` | `10`     | Maximum number of results to return.                                                           |
| `--top-k`       | `integer` | `3`      | Maximum number of chunks to return per note.                                                   |
| `--section`     | `string`  | _(None)_ | Filter results by heading path.                                                                |
| `--tag`         | `string`  | _(None)_ | Filter results by tag name (prefixed `#` is optional).                                         |
| `--has-tasks`   | `boolean` | `false`  | Only return chunks containing markdown task lists (`- [ ]`).                                   |
| `--has-code`    | `boolean` | `false`  | Only return chunks containing code blocks.                                                     |
| `--interactive` | `boolean` | `false`  | Launch a live interactive search TUI where you can type queries and preview results.           |
| `--split`       | `boolean` | `false`  | Split query string by delimiters (comma, pipe, semicolon) or execute multi-positional queries. |
| `--split-by`    | `string`  | `,       | ;`                                                                                             | Delimiters used to split query strings when `--split` is active. |

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

Reconstructs and displays the complete markdown text of a note by joining all of its indexed chunks. To ensure structural continuity and human readability, each chunk is automatically prepended with its dynamic Markdown section heading derived from its `heading_path` metadata (`### Section Heading\n\n<text>`).

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

Finds all notes linking to the target note using the local Wikilink graph. Link target resolution is fully canonicalized (`#anchor` headings stripped, subfolders resolved accurately against canonical paths), ensuring robust discovery even across deeply nested vault hierarchies.

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

Discovers "hidden" semantic connections: notes that are semantically similar but do not have direct Wikilinks in Obsidian. Supports granular `--deep` chunk-by-chunk analysis to identify exact matching sections between notes without requiring whole-note embedding comparisons.

#### Usage

```bash
notebrain hidden <note> [flags]
```

#### Arguments

- `<note>` _(required)_: The target note title, filename, or slug (e.g., `"SPIFFE"` or `"Rust Programming"`).

#### Command-Specific Flags

| Flag               | Type      | Default | Description                                                                                                                          |
| :----------------- | :-------- | :------ | :----------------------------------------------------------------------------------------------------------------------------------- |
| `--deep`           | `boolean` | `false` | Perform granular chunk-by-chunk analysis across individual note sections using stored vectors.                                       |
| `--include-linked` | `boolean` | `false` | Include notes that are already linked directly/indirectly in the hidden connections output while strictly excluding self-references. |
| `--top-k`          | `integer` | `3`     | Maximum matching target sections to evaluate and display per candidate note (in `--deep` mode).                                      |
| `--limit`          | `integer` | `10`    | Maximum number of hidden connections to return.                                                                                      |

#### Examples

```bash
# Discover 5 closest semantic notes to "Redis" that are not linked
notebrain hidden "Redis" --limit 5

# Perform deep chunk-by-chunk hidden connection discovery across sections of "SPIFFE"
notebrain hidden "SPIFFE" --deep --limit 3
```

---

### `tags`

Finds notes by tag name (default), or finds notes sharing tags with a given note (when using `--shared`).

#### Usage

```bash
notebrain tags <query> [flags]
```

#### Arguments

- `<query>` _(required)_: The tag name to search for (e.g. `#kubernetes` or `kubernetes`), or a note slug/title if `--shared` is used.

#### Command-Specific Flags

| Flag           | Type      | Default | Description                                                                                               |
| :------------- | :-------- | :------ | :-------------------------------------------------------------------------------------------------------- |
| `--shared`     | `boolean` | `false` | Treat the query as a note slug/title to find other notes sharing its tags.                                |
| `--children`   | `boolean` | `false` | Include child tags in hierarchical structure (e.g. searching 'kubernetes' also matches 'kubernetes/cka'). |
| `--min-shared` | `integer` | `1`     | Minimum number of shared tags to include a result (only applies when --shared is active).                 |

#### Examples

```bash
# Find all notes tagged with #kubernetes (auto-normalizes casing and # prefix)
notebrain tags "#Kubernetes"

# Find all notes tagged with #kubernetes and its child tags (e.g. #kubernetes/cka)
notebrain tags "kubernetes" --children

# Find notes sharing at least 2 tags with the note "redis-cluster"
notebrain tags "redis-cluster" --shared --min-shared 2
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

Using output formats like JSON and extracting fields via `--jsonpath` allows easy piping to shell tools and AI agents:

```bash
# Extract the slug of the top result
TOP_SLUG=$(notebrain search "golang channels" --limit 1 --jsonpath="$.results[0].note_slug")

# Pass it to fetch full content
notebrain get "$TOP_SLUG" --jsonpath="$.text"
```
