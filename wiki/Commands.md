# Commands Reference

NoteBrain has commands to ingest, query, and analyze your Obsidian vault.

```text
Usage: notebrain <command> [flags]

Index and search your Obsidian vault with semantic intelligence.

NoteBrain uses local LLM embeddings to index your Markdown notes into ChromaDB,
enabling powerful semantic search, hidden graph connections, and AI-friendly
automation workflows.
```

---

## Global Flags

You can apply these flags to `notebrain` before a subcommand (for example, `notebrain --verbose search "query"`). You can also put them in your configuration file.

| Flag                | Type      | Default                           | Description                                                                                                  |
| :------------------ | :-------- | :-------------------------------- | :----------------------------------------------------------------------------------------------------------- |
| `--config`          | `string`  | `~/.notebrain/config/config.toml` | The path to the TOML configuration file.                                                                                |
| `--chroma-path`     | `string`  | `~/.notebrain/chroma`             | The path to the persistent storage of ChromaDB. You can also use the `$CHROMA_PATH` environment variable.            |
| `--vault-path`      | `string`  | _(None)_                          | **Required.** The absolute path to your Obsidian vault.                                                          |
| `--vault-name`      | `string`  | _(Basename of vault)_             | The name of the Obsidian vault (to generate `obsidian://` URI links).                                           |
| `--debug`           | `boolean` | `false`                           | Enables the debug-level log output to stderr.                                                                 |
| `--format`          | `string`  | `text`                            | The output format: `text` (standard text), `json` (structured JSON), or `tsv` (tab-separated values).     |
| `--jsonpath`        | `string`  | _(None)_                          | A JSONPath expression to extract and filter fields from the JSON output (for example, `$.results[0].note_slug`). |
| `--include-text`    | `boolean` | `false`                           | Includes the text of the matched chunk inside structured outputs (`json`, `tsv`).             |
| `--context-window`  | `integer` | `0`                               | Gets ±N adjacent chunks around each match to give more context.                                  |
| `--min-score`       | `float`   | `0.0`                             | Does not show search results with a similarity score below this value (0.0 to 1.0).                                            |
| `--respect-exclude` | `boolean` | `false`                           | Obeys the Obsidian user filters and attachment exclusions during ingestion.                      |
| `--show-tags`       | `boolean` | `false`                           | Includes tag names (`#Tag/Subtag`) in the search and graph outputs.                                               |
| `--show-file-path`  | `boolean` | `true`                            | Includes `file_path` in outputs. You can use `--show-file-path=false` to omit it.                                           |
| `--version`         | `boolean` | `false`                           | Shows version information.                                                                                    |

> Note: Supported terminal emulators (for example, Ghostty, WezTerm, Kitty, and iTerm2) automatically enable clickable OSC 8 terminal links. You can set the `NO_HYPERLINKS=1` environment variable to disable hyperlinks for all commands.

### Token Efficiency and Quiet Mode for AI Agents

When you execute NoteBrain queries inside AI agent workflows, automated pipelines, or background scripts, you must control the token count. You must also hide interactive formatting.

1. **Clean stdout in machine formats**:
   NoteBrain writes query results to stdout and all diagnostics (progress, warnings) to stderr. When you use a non-interactive machine format (`--format=json`, `tsv`, or `--jsonpath`), stdout is already clean and correct for JSON parsers and AI agents.
2. **Compact JSON envelopes**:
   By default, the JSON output includes the necessary properties in a clean format. You can use `--show-file-path=false` to remove file paths. This decreases the token consumption for Large Language Models. The similarity scores (`score`) round to 4 decimal places (`0.8520`). The query headers (`query`) do not have terminal decorations.
3. **Non-redundant context windows (`--context-window N`)**:
   If you pass `--context-window N` (for example, `--context-window 1` or `2`) with `--include-text`, NoteBrain gets ±N adjacent chunks into the `context` array. It does not include the matched chunk (`text`) in the array (`PopulateContext`). This prevents duplicated text across `text` and `context`.

---

### Context-Aware Empty Result Guidance

When a search or graph command returns zero results in standard terminal `text` format (`--format=text`), NoteBrain does not show generic `(no results)` text. Instead, it shows helpful tips in italicized amber (`hintStyle`) under the command header:

- **`backlinks`**: Tells you to see if other notes link to the target, or to run `notebrain ingest` again.
- **`connections`**: Tells you to increase `--hops` or to make sure that the Wikilinks are correct.
- **`hidden`**: Tells you to use `--include-linked` to include notes that can already have links. It also tells you to run the index again if the note is too unique.
- **`tags`**: Tells you to check the note tags or to decrease `--min-shared`.
- **`search` / `boosted`**: Tells you to use broader search terms, to change `--boost`, or to run `notebrain ingest`.

> Note: Contextual hints show only in standard `text` output. To maintain compatibility with automated scripts and AI agents, machine formats (`json`, `tsv`, `--jsonpath`) omit these hints.

---

## Note Resolution (`<note>` argument)

Commands that target a specific note (`backlinks`, `connections`, `hidden`, `tags`, `boosted --seed=<note>`, `get`) accept these formats for the `<note>` parameter:

1. **Exact note slug**: The normalized identifier in the database (for example, `00fleeting-noteskubernetes-networking-toolsspiffe`).
2. **Note title**: The note title. This does not care about case (for example, `"SPIFFE"` or `"Rust Programming"`).
3. **Filename**: The exact file name. This does not care about case (for example, `"SPIFFE.md"`).
4. **Partial path or suffix**: The end of the relative path inside your vault (for example, `"tools/SPIFFE.md"`).

If multiple notes have the same title or filename in different directories, NoteBrain returns an ambiguity error. This error shows a list of the candidate slugs. You must then supply the exact path or slug.

---

## Command Reference

### `ingest`

This command indexes markdown files from your Obsidian vault. It parses Wikilinks and tags. It divides the contents into chunks. Then it makes local vector embeddings.

#### Usage

```bash
notebrain ingest [<glob>] [flags]
```

#### Arguments

- `[<glob>]` (optional): A glob pattern to specify files or folders to ingest (for example, `Projects/**`).

#### Command-Specific Flags

| Flag                | Type      | Default | Description                                                                             |
| :------------------ | :-------- | :------ | :-------------------------------------------------------------------------------------- |
| `--workers`         | `integer` | `4`     | The number of concurrent ingestion workers.                                                 |
| `--min-chunk-words` | `integer` | `10`    | Does not include chunks that have fewer words than this value.                                                 |
| `--chunk-size`      | `integer` | `800`   | The maximum number of runes per chunk for the parser.                                                 |
| `--chunk-overlap`   | `integer` | `100`   | The number of overlap runes between sub-chunks when the parser splits a section.                               |
| `--enable-pdf`      | `boolean` | `false` | Enables the extraction of PDF text. This requires `--llm-model`.                      |
| `--llm-model`       | `string`  | `""`    | The LLM model to parse PDFs (for example, `openrouter/inclusionai/ling-3.0-flash:free`). |
| `--llm-context-window` | `integer` | `128000` | The total context window size of the LLM in tokens.                                         |

> Note: During the index procedure, the tool excludes attachment links from the graph edges.

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

This command does a semantic vector search across all the indexed chunks in your vault. You can filter the results by sections, tags, tasks, and code.

#### Usage

```bash
notebrain search [<query>] [flags]
```

#### Arguments

- `[<query>]` (optional): The semantic query string. You can supply multiple query strings as positional arguments for multi-hit boosting. If you specify `--tag`, you can omit this argument.

#### Command-Specific Flags

| Flag          | Type      | Default  | Description                                                 |
| :------------ | :-------- | :------- | :---------------------------------------------------------- |
| `--limit`     | `integer` | `10`     | The maximum number of results to show.                        |
| `--top-k`     | `integer` | `3`      | The maximum number of chunks to show for each note.                |
| `--section`   | `string`  | _(None)_ | Filters the results by the heading path.                             |
| `--tag`       | `string`  | _(None)_ | Filters the results by the tag name (the `#` prefix is optional).      |
| `--has-tasks` | `boolean` | `false`  | Shows only the chunks that contain markdown task lists (`- [ ]`).|
| `--has-code`  | `boolean` | `false`  | Shows only the chunks that contain code blocks.                  |
| `--with-pdf`  | `boolean` | `false`  | Includes the PDF results in the search (the default is markdown only). |

#### Examples

```bash
# Basic semantic search
notebrain search "reconciliation loop in kubernetes" --limit 5

# Search specifically for tasks under the Kubernetes tag
notebrain search "deploy service" --tag "Kubernetes" --has-tasks

# Multi-query search with multi-hit boosting (positional arguments)
notebrain search "message brokers" "redis queue"

# Search showing tags in output
notebrain search "redis streams" --show-tags
```

#### How Multi-Query Matching and Ranking Works

When you supply multiple query positional arguments (`notebrain search "arg1" "arg2"`):

1. **Semantic vector matching**: NoteBrain puts each query independently into a 384-dimensional vector with `MiniLM-L6-v2`. The matching depends entirely on the semantic vector similarity (cosine distance in ChromaDB). It does not use exact keyword or substring matching. A note can match a query when it uses different words or synonyms.
2. **Multi-hit boosting**: If a note chunk matches multiple queries in your search, NoteBrain increases its rank. The tool sorts the results with a two-tier strategy:
   - **Primary sort**: Descending order by the number of matched query topics (`len(MatchedQueries)`). Chunks that connect multiple concepts (for example, chunks that match `"message brokers"` and `"redis queue"`) show at the top.
   - **Secondary sort**: Descending order by the maximum cosine similarity score in each match-count tier.
3. **Hit attribution**: In the terminal text mode, multi-hit chunks show attribution tags. These tags tell you which query vectors found them (for example, `[hits: "message brokers", "redis queue"]`). In structured JSON outputs, each item has a `matched_queries` array.

---

### `get`

This command builds and shows the complete markdown text of a note. It joins all the indexed chunks of the note. To make the text clear and easy to read, the tool automatically puts a dynamic markdown section heading before each chunk. It gets this heading from the `heading_path` metadata (`### Section Heading\n\n<text>`).

#### Usage

```bash
notebrain get <slug> [flags]
```

#### Arguments

- `<slug>` (required): The note slug (for example, `kubernetes-native-applications`) or the vault file path.

#### Examples

```bash
# Retrieve full content of a note by slug
notebrain get "kubernetes-native-applications"

# Retrieve full content of a note, outputting to JSON
notebrain get "kubernetes-native-applications" --format json
```

---

### `backlinks`

This command finds all the notes that link to the target note. It uses the local Wikilink graph. The link target resolution is fully canonicalized. This means that the tool removes `#anchor` headings and resolves subfolders against canonical paths. This makes sure that the tool finds connections across deeply nested vault hierarchies.

#### Usage

```bash
notebrain backlinks <note> [flags]
```

#### Arguments

- `<note>` (required): The target note slug or title.

#### Examples

```bash
# Find what notes link to "Redis"
notebrain backlinks "Redis"
```

---

### `connections`

This command does a breadth-first traversal of the Wikilink graph. It finds connected notes up to a specified number of hops.

#### Usage

```bash
notebrain connections <note> [flags]
```

#### Arguments

- `<note>` (required): The starting note slug or title.

#### Command-Specific Flags

| Flag     | Type      | Default | Description                               |
| :------- | :-------- | :------ | :---------------------------------------- |
| `--hops` | `integer` | `2`     | The maximum number of graph hops to traverse. |

#### Examples

```bash
# Find notes connected within 2 hops of "Redis"
notebrain connections "Redis" --hops 2
```

---

### `hidden`

This command finds hidden semantic connections. These are notes that are semantically similar, but do not have direct Wikilinks in Obsidian. You can use the `--deep` flag for a chunk-by-chunk analysis. This analysis finds exact matching sections between notes. It does not require whole-note embedding comparisons.

#### Usage

```bash
notebrain hidden <note> [flags]
```

#### Arguments

- `<note>` (required): The target note title, filename, or slug (for example, `"SPIFFE"` or `"Rust Programming"`).

#### Command-Specific Flags

| Flag               | Type      | Default | Description                                                                                                                          |
| :----------------- | :-------- | :------ | :----------------------------------------------------------------------------------------------------------------------------------- |
| `--deep`           | `boolean` | `false` | Does a chunk-by-chunk analysis across individual note sections with the stored vectors.                                       |
| `--include-linked` | `boolean` | `false` | Includes notes that already have direct or indirect links in the hidden connections output. This strictly excludes self-references. |
| `--top-k`          | `integer` | `3`     | The maximum number of matching target sections to evaluate and show for each candidate note (in `--deep` mode).                                      |
| `--limit`          | `integer` | `10`    | The maximum number of hidden connections to show.                                                                                      |

#### Examples

```bash
# Discover 5 closest semantic notes to "Redis" that are not linked
notebrain hidden "Redis" --limit 5

# Perform deep chunk-by-chunk hidden connection discovery across sections of "SPIFFE"
notebrain hidden "SPIFFE" --deep --limit 3
```

---

### `tags`

This command finds notes by the tag name (the default). If you use `--shared`, it finds notes that share tags with a given note.

#### Usage

```bash
notebrain tags <query> [flags]
```

#### Arguments

- `<query>` (required): The tag name to search for (for example, `#kubernetes` or `kubernetes`). If you use `--shared`, supply a note slug or title.

#### Command-Specific Flags

| Flag           | Type      | Default | Description                                                                                               |
| :------------- | :-------- | :------ | :-------------------------------------------------------------------------------------------------------- |
| `--shared`     | `boolean` | `false` | Uses the query as a note slug or title to find other notes that share its tags.                                |
| `--children`   | `boolean` | `false` | Includes child tags in the hierarchical structure (for example, a search for 'kubernetes' also finds 'kubernetes/cka'). |
| `--min-shared` | `integer` | `1`     | The minimum number of shared tags necessary to show a result (only when `--shared` is active).                 |

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

This command does a graph-boosted semantic search. It combines the semantic vector similarity with the Wikilink graph distance from a seed note. This increases the similarity scores for notes that are structurally connected to the seed.

#### Usage

```bash
notebrain boosted <query> --seed=STRING [flags]
```

#### Arguments

- `<query>` (required): The search query.

#### Command-Specific Flags

| Flag      | Type      | Default  | Description                                                     |
| :-------- | :-------- | :------- | :-------------------------------------------------------------- |
| `--seed`  | `string`  | _(None)_ | **Required.** The origin note slug or title for graph boosting. |
| `--boost` | `float`   | `1.5`    | The score multiplier for graph-connected results (for example, 1.5 = 50% boost). |
| `--limit` | `integer` | `10`     | The maximum number of results to show.                            |
| `--with-pdf`| `boolean` | `false`  | Includes the PDF results in the search (the default is markdown only).   |

#### Examples

```bash
# Perform search boosted by structural connections to "Redis"
notebrain boosted "caching strategies" --seed "Redis" --boost 2.0 --limit 5
```

---

### `doctor`

This command runs a diagnostic health check on your environment. It makes sure that NoteBrain has the correct configuration and can access the necessary dependencies.

Checks performed:

- **Vault Path** — the configured vault directory exists and is accessible.
- **ChromaDB Path** — the database directory is writable.
- **ChromaDB sqlite** — `chroma.sqlite3` exists, has a valid SQLite header, and a sane file size.
- **ChromaDB index** — each collection segment has all of its HNSW index files (missing or empty files mean an interrupted write).
- **ChromaDB open test** — opens the database in a subprocess and forces the HNSW indexes to load. A corrupted native index aborts the subprocess; the doctor reports the signal and suggests `notebrain reset`.

The command exits non-zero when database problems are found.

#### Usage

```bash
notebrain doctor [flags]
```

---

### `init`

This command starts an interactive wizard to create or update your `config.toml` file. It automatically finds your vault path. It lets you enable or disable PDF support.

#### Usage

```bash
notebrain init [flags]
```

---

### `stats`

This command shows statistics for your NoteBrain collection (the total number of indexed chunks and links).

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

This command drops all the NoteBrain collections (`nb_chunks` and `nb_links`) and starts fresh. You cannot reverse this operation.

#### Usage

```bash
notebrain reset [flags]
```

> Note: For automated scripts, you can pipe `yes` to skip the interactive confirmation prompt:

```bash
echo yes | notebrain reset
```

---

### `version`

This command prints version information. This information includes the build commit hash and the compile date.

#### Usage

```bash
notebrain version [flags]
```

---

## Configuration File

You can persistently set any global flag in `~/.notebrain/config/config.toml` (or a custom path that you give to `--config`). Keys in the configuration file support interchangeable `kebab-case` and `snake_case` styles.

```toml
# ~/.notebrain/config/config.toml
vault-path = "/home/user/Obsidian/MainVault"
vault-name = "MainVault"
chroma-path = "~/.notebrain/chroma"
format = "json"
debug = true
show-tags = false
```

---

## Machine-Readable Output and AI Chain Automation

You can use output formats like JSON and extract fields with `--jsonpath`. This lets you easily pipe the output to shell tools and AI agents:

```bash
# Extract the slug of the top result
TOP_SLUG=$(notebrain search "golang channels" --limit 1 --jsonpath="$.results[0].note_slug")

# Pass it to fetch full content
notebrain get "$TOP_SLUG" --jsonpath="$.text"
```
