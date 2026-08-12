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

You can apply these flags to `notebrain` before a subcommand (for example, `notebrain --debug search "query"`). You can also put them in your configuration file.

> Note: In `notebrain <command> --help` output, the flags are grouped under titled sections: `Global Flags` (shared by all commands), the command-specific section (for example, `Search Flags`), and `Output Flags` (`--include-text`, `--context-window`, `--min-score`).

| Flag                | Type      | Default                           | Description                                                                                                  |
| :------------------ | :-------- | :-------------------------------- | :----------------------------------------------------------------------------------------------------------- |
| `--config`          | `string`  | `~/.notebrain/config/config.toml` | The path to the TOML configuration file.                                                                                |
| `--chroma-path`     | `string`  | `~/.notebrain/chroma`             | The path to the persistent storage of ChromaDB.                                                                             |
| `--vault-path`      | `string`  | _(None)_                          | **Required.** The absolute path to your Obsidian vault.                                                          |
| `--vault-name`      | `string`  | _(Basename of vault)_             | The name of the Obsidian vault (to generate `obsidian://` URI links).                                           |
| `--log-level`       | `string`  | `info`                            | The logging severity level: `debug`, `info`, `warn`, or `error`. You can set it with the `NOTEBRAIN_LOG_LEVEL` environment variable. Flag and config file values take precedence over the environment variable. |
| `--debug`           | `boolean` | `false`                           | Enables the debug-level log output to stderr. This is a legacy alias for `--log-level=debug`.                    |
| `--log-file`        | `string`  | _(None)_                          | Writes logs to this file (JSON) in addition to stderr. The file rotates on size. You can set it with the `NOTEBRAIN_LOG_FILE` environment variable. Flag and config file values take precedence over the environment variable. |
| `--log-max-size-mb` | `integer` | `10`                              | The max size of each log file in MiB before rotation (`0` uses the default `10`).                                  |
| `--log-max-backups` | `integer` | `5`                               | The number of rotated log file backups to keep (`0` uses the default `5`). Rotated files are named `<file>.1`, `<file>.2`, and so on. |
| `--skip-phantom`    | `boolean` | `true`                            | Excludes phantom (uncreated) notes from the results. Use `--skip-phantom=false` to include them.                 |
| `--format`          | `string`  | `text`                            | The output format: `text` (standard text), `json` (structured JSON), or `tsv` (tab-separated values). |

> Note: In `tsv` output, the `text` field escapes tabs and line breaks (as `\t` and `\n`). This keeps every result on a single line for line-based parsers. Set `--include-text=false` to omit the `text` field entirely.

### TSV Column Layout

The result-list commands (`search`, `hidden`, `tags`, `boosted`, `backlinks`, `connections`) print this header:

```text
slug	title	file_path	score	tags	extra	heading_path	text
```

`get` prints this header:

```text
note_slug	title	file_path	tags	chunks	text
```

`stats` prints this header:

```text
notes	chunks	links
```
| `--jsonpath`        | `string`  | _(None)_                          | A JSONPath expression to extract and filter fields from the JSON output (for example, `$.results[0].note_slug`). |
| `--show-tags`       | `boolean` | `false`                           | Includes tag names (`#Tag/Subtag`) in the search and graph outputs.                                               |
| `--show-file-path`  | `boolean` | `true`                            | Includes `file_path` in outputs. You can use `--show-file-path=false` to omit it.                                           |
| `--version`         | `boolean` | `false`                           | Shows version information.                                                                                    |

### Shared Query Flags

The `search`, `hidden`, `boosted`, and `tags` commands accept these flags:

| Flag               | Type      | Default | Description                                                                 |
| :----------------- | :-------- | :------ | :-------------------------------------------------------------------------- |
| `--include-text`   | `boolean` | `false` | Includes the text of the matched chunk inside structured outputs (`json`, `tsv`). |
| `--context-window` | `integer` | `0`     | Gets ±N adjacent chunks around each match to give more context.             |
| `--min-score`      | `float`   | `0.0`   | Does not show search results with a similarity score below this value (0.0 to 1.0). |

> Note: Supported terminal emulators (for example, Ghostty, WezTerm, Kitty, and iTerm2) automatically enable clickable OSC 8 terminal links. You can set the `NO_HYPERLINKS=1` environment variable to disable hyperlinks for all commands.

> Note: The human-readable `text` output uses colors only when stdout is a terminal. When you pipe or redirect the output (for example, `notebrain stats > file.txt`), the colors are disabled automatically. Set `NO_COLOR=1` or `TERM=dumb` to disable colors on a terminal. This keeps the text output clean for scripts and file capture.

> Note: A command exits with a non-zero status when the JSONPath expression is invalid. Scripts can use the exit status to detect this error.

### Exit Codes

NoteBrain uses stable exit codes. Automation and scripts can use them to tell usage errors from operational failures.

| Code | Meaning                                                                                          |
| :--- | :------------------------------------------------------------------------------------------------ |
| `0`  | The command ran successfully.                                                                    |
| `1`  | The command failed at runtime (for example, a missing vault, a corrupted database, or an API error). |
| `2`  | The command-line arguments were invalid (for example, an unknown flag, a missing query, a missing `--vault-path`, or an invalid `--log-level` value). |

### Logging Behavior

NoteBrain writes all logs to stderr. It never writes logs to stdout. This keeps stdout clean for command results and for machine formats.

- **Formats**: NoteBrain writes JSON logs when stderr is not a terminal (for example, in scripts, cron, or systemd). It writes text logs when stderr is a terminal.
- **Colors**: On a terminal, the log level label (`INFO`, `WARN`, `ERROR`, `DEBUG`) is colored. Set the `NO_COLOR` environment variable (or use `TERM=dumb`) to disable colors.
- **Log files**: Set `--log-file` (or `NOTEBRAIN_LOG_FILE`) to write logs to a rotating JSON file in addition to stderr. `--log-max-size-mb` and `--log-max-backups` control rotation. The systemd service and cron examples in `contrib/automation/` capture stderr, so `--log-file` is optional for them.
- **Panic safety**: If NoteBrain hits an internal error, it prints `internal error: ...` to stderr, logs the stack, and exits with code `1`. It does not dump a raw stack trace.

### Token Efficiency and Quiet Mode for AI Agents

When you execute NoteBrain queries inside AI agent workflows, automated pipelines, or background scripts, you must control the token count. You must also hide interactive formatting.

1. **Clean stdout in machine formats**:
   NoteBrain writes query results to stdout and all diagnostics (progress, warnings) to stderr. When you use a non-interactive machine format (`--format=json`, `tsv`, or `--jsonpath`), stdout is clean and correct for JSON parsers and AI agents.
 2. **Compact JSON envelopes**:
     By default, the JSON output includes the necessary properties in a clean format. You can use `--show-file-path=false` to remove file paths. This decreases the token consumption for Large Language Models. In JSON output, the similarity scores (`score`) round to 4 decimal places (`0.8520`). In TSV output, they keep 6 decimal places (`0.852000`). The query headers (`query`) do not have terminal decorations.
    All commands wrap their JSON output in the same envelope: the command name under `command`, the inputs under `query`, and the data under a command-specific key (`results`, `note`, or the count fields). For example, `notebrain get "slug" --format=json` returns `{"command":"get","query":"slug","note":{...}}`. The `--jsonpath` expression still operates on the inner data, so existing paths such as `$.note_slug` and `$.links` keep working.
3. **Non-redundant context windows (`--context-window N`)**:
   If you pass `--context-window N` (for example, `--context-window 1` or `2`) with `--include-text`, NoteBrain gets ±N adjacent chunks into the `context` array. It does not include the matched chunk (`text`) in the array (`PopulateContext`). This prevents duplicated text across `text` and `context`.

---

### Context-Aware Empty Result Guidance

When a search or graph command returns zero results in standard terminal `text` format (`--format=text`), NoteBrain does not show generic `(no results)` text. Instead, it shows helpful tips in italicized amber (`hintStyle`) under the command header:

- **`backlinks`**: Tells you to run `notebrain ingest` again, or to make sure that other notes link to the target.
- **`connections`**: Tells you to increase `--hops` or to make sure that the Wikilinks are correct.
- **`hidden`**: Tells you to use `--include-linked` to include notes that can already have links. It also tells you to run the index again if the note is too unique.
- **`tags`**: Tells you to see the note tags or to decrease `--min-shared`.
- **`search` / `boosted`**: Tells you to use broader search terms, to change `--boost`, or to run `notebrain ingest`.

> Note: Contextual hints show only in standard `text` output. To maintain compatibility with automated scripts and AI agents, machine formats (`json`, `tsv`, `--jsonpath`) omit these hints.

---

## Note Resolution (`<note>` argument)

Commands that target a specific note (`backlinks`, `connections`, `hidden`, `tags`, `boosted --seed=<note>`, `get`) accept these formats for the `<note>` parameter:

1. **Exact note slug**: The normalized identifier in the database (for example, `kubernetes-networking-tools-spiffe`).
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
| `--llm-model`       | `string`  | `""`    | The LLM model to parse PDFs (for example, `openrouter/anthropic/claude-3.5-haiku`). |
| `--llm-context-window` | `integer` | `128000` | The total context window size of the LLM in tokens.                                         |
| `--respect-exclude` | `boolean` | `false` | Obeys the Obsidian user filters and attachment exclusions during ingestion. |

#### Attachment Embeds

An attachment embed shows a file inside a note (for example, `![[diagram.webp]]`). The tool never adds an attachment embed to the graph edges. The embed includes the file content. It does not reference a note.

The chunk text keeps two forms of the embed:

- **Rich form**: The original wikilink (for example, `![[diagram.webp|200x150]]`). Commands such as `notebrain get` and `--include-text` show this form.
- **Embedding form**: A marker without the filename. The vector model reads this form. Image embeds become `[image]`. Other attachments become `[attachment]`. A text label stays in the marker (for example, `[image: Architecture diagram]`). Numeric size suffixes (for example, `|200x150`) do not appear.

> Note: The chunk schema version is `5`. After an upgrade, run `notebrain ingest` again. The index procedure rebuilds the chunks with the embedding form.

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

This command does a semantic vector search across all the indexed chunks in your vault. You can filter the results by sections, tags, tasks, and code, and exclude specific notes from the results.

#### Usage

```bash
notebrain search [<query>] [flags]
```

#### Arguments

- `[<query>]` (optional): The semantic query string. You can supply multiple query strings as positional arguments for multi-hit boosting. If you specify `--tag`, you can omit this argument. You can also pipe the query through stdin, for example `echo "my query" | notebrain search`.

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
| `--exclude-note` | `string` | _(None)_ | Excludes notes from the results. Accepts a note slug, title, or path; repeat the flag or use comma-separated values to exclude multiple notes. |

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

# Exclude private and archive notes from results (slug, title, or path)
notebrain search "reconciliation loop in kubernetes" --exclude-note "private/daily-journal" --exclude-note "archive"
notebrain search "redis queues" --exclude-note "zeta-note.md,beta.md"

# Text output notes that some notes were excluded
notebrain search "kubernetes" --exclude-note "archive"
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

This command builds and shows the complete markdown text of a note. It joins all the indexed chunks of the note. To make the text readable, the tool puts a dynamic markdown section heading before each chunk. It gets this heading from the `heading_path` metadata (`### Section Heading\n\n<text>`).

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

### `refs`

This command lists the direct references of a note: local attachment files (images, PDFs, archives, …) and external website links. It parses the note file fresh from the vault, so it sees everything, including embedded images that the index intentionally skips. References are listed in first-occurrence order, deduplicated by resolved path (or exact URL).

The command resolves the note exactly like the other note commands (slug, title, filename, or partial path), then prints absolute file paths and URLs, filterable by kind.

Reference resolution follows Obsidian semantics:

- **Wiki links** (`[[file.png]]`): targets containing a folder path start at the vault root; `./` targets resolve relative to the note's folder; bare names search the note's folder, then the vault root, then the configured `attachmentFolderPath` from `.obsidian/app.json`.
- **Markdown links** (`[doc](file.pdf)`, `![alt](img.png)`): resolve relative to the note's folder, with percent-decoding (`Router%20Modes.webp` → `Router Modes.webp`) and traversal protection (`..` escapes are rejected).

Broken links are hidden by default; pass `--include-missing` to list them marked `missing: true`. External links are never verified (the tool is offline by design), so URL rows always report `missing: false`.

#### Usage

```bash
notebrain refs <note> [flags]
```

#### Arguments

- `<note>` (required): The note slug, title, or file path (auto-resolved).

#### Command-Specific Flags

| Flag | Description |
| --- | --- |
| `--images` | Include image attachments only |
| `--pdf` | Include PDF attachments only |
| `--other` | Include other attachments (video, audio, archives, office docs) |
| `--external-links` | Include external website links (URLs) only |
| `--include-missing` | Include references whose file is missing from the vault (marked `missing: true`) |

Filters combine with OR semantics; with no filter flags every kind is listed.

#### Examples

```bash
# List every reference of a note
notebrain refs "kubernetes-notes"

# List image attachments as machine-readable JSON
notebrain refs "kubernetes-notes" --images --format=json

# List PDF attachments as TSV
notebrain refs "kubernetes-notes" --pdf --format=tsv

# List external website links
notebrain refs "kubernetes-notes" --external-links --format=json

# Feed attachment paths straight into a script
notebrain refs "$SLUG" --images --jsonpath='$.refs[*].path'
```

#### JSON shape

```json
{
  "command": "refs",
  "note_slug": "kubernetes-notes",
  "title": "Kubernetes Notes",
  "total": 3,
  "refs": [
    {"path": "/vault/assets/arch.png", "relative_path": "assets/arch.png", "kind": "image", "missing": false},
    {"path": "/vault/99.Storage-Shed/Attachments/guide.pdf", "relative_path": "99.Storage-Shed/Attachments/guide.pdf", "kind": "pdf", "missing": false},
    {"path": "https://example.com/docs", "kind": "external-links", "missing": false}
  ]
}
```

External rows omit `relative_path`. TSV output uses the header `path\tkind\tmissing\trelative_path`; external rows leave `missing` and `relative_path` empty.

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

This command finds hidden semantic connections. These are notes that are semantically similar, but do not have direct Wikilinks in Obsidian. You can use the `--deep` flag for a chunk-by-chunk analysis. This analysis finds exact matching sections between notes. It does not require whole-note embedding comparisons. For the exact ranking rules of `--deep`, read the [Ranking](Ranking.md) document.

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
| `--candidate-chunks` | `integer` | _(None)_ | The maximum number of matching target sections to evaluate and show for each candidate note (in `--deep` mode). Replaces `--top-k`. |
| `--top-k`          | `integer` | `3`     | A deprecated alias for `--candidate-chunks`.                                                                                                      |
| `--limit`          | `integer` | `10`    | The maximum number of hidden connections to show.                                                                                      |

#### How `--deep` Ranking Works

The `--deep` mode does not rank candidates by the whole-note similarity score. It ranks them by the breadth of section overlap. The tool runs one vector query per chunk of the seed note. It counts how many distinct seed-note sections matched each candidate. The primary sort key is this count, descending. The secondary sort key is the best similarity score, descending.

The tool shows a `Matched target sections (N)` tag for each candidate. This number counts only the sections that passed the score thresholds. The thresholds remove weak matches. The shown number can be lower than the true number of matching sections. For example, a note can match 15 sections, but the tool shows only 1. A score close to 1.0 shows all sections. A low score shows only the best one.

The top candidates on the list are often those with the widest section overlap, not the highest single score. An exact single-section match can outrank a candidate with one strong section.

For the full flow, read the [Ranking](Ranking.md) document.

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
| `--for-note`   | `boolean` | `false` | An alias for `--shared`.                                                                                  |
| `--children`   | `boolean` | `false` | Includes child tags in the hierarchical structure (for example, a search for 'kubernetes' also finds 'kubernetes/cka'). |
| `--min-shared` | `integer` | `1`     | The minimum number of shared tags necessary to show a result (only when `--shared` or `--for-note` is active).                 |
| `--limit`      | `integer` | `50`    | The maximum number of results to show.                                                                        |

`tags` also accepts the shared output flags `--include-text`, `--context-window`, and `--min-score`.

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
- **ChromaDB open test** — opens the database in a subprocess and forces the HNSW indexes to load. A corrupted native index aborts the subprocess. The doctor reports the signal and suggests `notebrain reset`.

The command exits non-zero when database problems are found.

#### Usage

```bash
notebrain doctor [flags]
```

---

### `init`

This command starts an interactive wizard to create or update your `config.toml` file. The wizard prefills your existing configuration, if one exists. It asks for the vault path. It can enable or disable PDF support.

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

#### Command-Specific Flags

| Flag    | Type      | Default | Description                                                                |
| :------ | :-------- | :------ | :------------------------------------------------------------------------- |
| `--yes` | `boolean` | `false` | Skips the confirmation prompt (short form: `-y`). Use it in scripts. |

> Note: For automated scripts, you can pipe `yes` to skip the interactive confirmation prompt:

```bash
echo yes | notebrain reset
```

You can also use the flag:

```bash
notebrain reset -y
```

---

### `version`

This command prints version information. This information includes the build commit hash and the compile date.

#### Usage

```bash
notebrain version [flags]
```

---

### `completion`

This command prints the activation code for tab completion. It supports **bash**, **zsh**, and **fish**. It completes subcommands, flags, enum values, and note slugs from your live index.

#### Usage

```bash
notebrain completion [bash|zsh|fish]
```

Without a shell argument, the command detects your login shell automatically. Use `-c` to print only the code that you source from your init file.

#### Examples

```bash
# Show the activation line for the current shell
notebrain completion

# Print the code for zsh to source from ~/.zshrc
notebrain completion -c zsh
```

For the full activation instructions and limitations, see [Shell Completion](Shell_Completion.md).

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

You can use output formats like JSON and extract fields with `--jsonpath`. You can pipe the output to shell tools and AI agents:

```bash
# Extract the slug of the top result
TOP_SLUG=$(notebrain search "golang channels" --limit 1 --jsonpath="$.results[0].note_slug")

# Pass it to fetch full content
notebrain get "$TOP_SLUG" --jsonpath="$.text"
```
