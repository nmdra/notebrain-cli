# NoteBrain CLI Flag Reference

This reference documents command-specific and global flags. Read this when you need precise default values, flag availability per command, or flag interactions that aren't covered in the main SKILL.md.

## Command-Specific Flags

These flags are available only on the commands listed.

### `search`

| Flag                 | Purpose                                                                                                               | Default  |
| -------------------- | --------------------------------------------------------------------------------------------------------------------- | -------- |
| `--limit N`          | Maximum total results to return.                                                                                      | `10`     |
| `--top-k N`          | Maximum chunks to retain **per note**. Prevents one long note from dominating results.                                | `3`      |
| `--split`            | Split query string by delimiters (comma, pipe, semicolon) for independent sub-searches with multi-hit score boosting. | off      |
| `--split-by "CHARS"` | Delimiter characters for `--split`.                                                                                   | `",\|;"` |
| `--section "PATH"`   | Filter results to chunks under a specific heading path (e.g., `"Architecture > Components"`).                         | —        |
| `--tag "TagName"`    | Filter results to notes with this tag.                                                                                | —        |
| `--has-tasks`        | Only return chunks containing task lists (checkboxes).                                                                | off      |
| `--has-code`         | Only return chunks containing fenced code blocks.                                                                     | off      |


### `hidden`

| Flag               | Purpose                                                                                                             | Default |
| ------------------ | ------------------------------------------------------------------------------------------------------------------- | ------- |
| `--limit N`        | Maximum number of hidden connections to return.                                                                     | `10`    |
| `--deep`           | Analyze each chunk individually for granular section-level matches using stored vectors (no re-embedding required). | `false` |
| `--top-k N`        | Chunks to evaluate per candidate note in `--deep` mode.                                                             | `3`     |
| `--include-linked` | Include notes that are already linked directly/indirectly, while still excluding self-references.                   | `false` |

### `connections`

| Flag       | Purpose                                                                                            | Default |
| ---------- | -------------------------------------------------------------------------------------------------- | ------- |
| `--hops N` | Breadth-first search traversal depth. Keep to 1–2 to avoid exponential blowup of returned results. | `2`     |

### `tags`

| Flag             | Purpose                                                                                                     | Default |
| ---------------- | ----------------------------------------------------------------------------------------------------------- | ------- |
| `--shared`       | Treat the query as a note slug/title to find notes sharing its tags.                                         | `false` |
| `--children`     | Include child tags in hierarchical structure (e.g. searching 'kubernetes' also matches 'kubernetes/cka').  | `false` |
| `--min-shared N` | Minimum number of shared tags required to include a result (only applies when --shared is active).           | `1`     |

### `boosted`

| Flag            | Purpose                                                                                 | Default |
| --------------- | --------------------------------------------------------------------------------------- | ------- |
| `--seed STRING` | **Required.** Seed note (slug, title, or path) whose graph neighbors get a score boost. | —       |
| `--limit N`     | Maximum number of results.                                                              | `10`    |
| `--boost F`     | Score multiplier for graph-connected results (e.g., `1.5` = 50% boost over base score). | `1.5`   |

### `get`

No command-specific flags. Takes a single positional argument: `<slug>` (note slug, title, or file path — auto-resolved).

## Global Flags (Available on All Query Commands)

These flags work identically on `search`, `backlinks`, `connections`, `hidden`, `tags`, `boosted`, `get`, and `stats`.

### Output Format & Extraction

| Flag              | Purpose                                                                                                                                           | Default |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--format FORMAT` | Output format: `json` (structured envelope), `ndjson` (one JSON object per line), `tsv` (tab-separated, no key names), `text` (standard text).         | `text`  |
| `--compact`       | Omit redundant envelope fields (`command`, `file_path`) from `json`/`ndjson` output. Reduces token footprint by ~40–50% — recommended for agents. | `false` |
| `--jsonpath PATH` | Extract specific JSON elements using JSONPath (e.g., `"$.results[*].note_slug"`). Eliminates JSON envelope overhead entirely.                     | —       |
| `--include-text`  | Include the matched markdown text chunk in results. Omit during initial structure-mapping to save tokens.                                         | off     |

### Search & Filtering

| Flag                                                                                                                 | Purpose                                                                                                                      | Default |
| -------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--context-window N`                                                                                                 | Fetch ±N adjacent chunks around each match into the `context` field. Use for lightweight surrounding context across results. | `0`     |
| `--min-score F`                                                                                                      | Suppress results below this similarity score (0.0–1.0).                                                                      | `0`     |
| `--hide-tags`                                                                                                        | Hide tag names (`#Tag/Subtag`) from output. Pass `--hide-tags=false` to show them.                                           | `true`  |
| `--skip-attachments`                                                                                                 | Exclude attachment and image links (e.g., `.webp`, `.png`, `.canvas`) from graph edges and backlinks.                        | `true`  |
| `--skip-phantom`                                                                                                     | Exclude uncreated notes (phantom wikilinks without a `.md` file on disk) from results.                                       | `true`  |
| The vault must have been indexed at least once via `notebrain ingest` before any query commands will return results. |

### Environment & Config

| Flag                  | Purpose                                                              | Default                           |
| --------------------- | -------------------------------------------------------------------- | --------------------------------- |
| `--chroma-path PATH`  | Path to ChromaDB persistent storage directory.                       | `~/.notebrain/chroma`             |
| `--vault-path PATH`   | Path to the Obsidian vault directory.                                | (from config)                     |
| `--vault-name STRING` | Vault display name for Obsidian URI links.                           | basename of `--vault-path`        |
| `--config PATH`       | Path to config file.                                                 | `~/.notebrain/config/config.toml` |

| `--verbose`           | Show detailed output including all matched sections.                 | off                               |
| `--no-hyperlinks`     | Disable clickable terminal hyperlinks in output.                     | off                               |

> [!TIP]
> **Persistent Compact Mode**: Add `compact = true` to `~/.notebrain/config/config.toml` to automatically apply `--compact` to all query commands. When active, JSON output omits `command` and `file_path`, rounds `score` to 4 decimal places, and strips `query` headers — retaining only `note_slug`, `title`, `score`, `text`, and `context`.
