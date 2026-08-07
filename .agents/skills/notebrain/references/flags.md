# NoteBrain CLI Flag Reference

This reference documents command-specific and global flags. Read this when you need precise default values, flag availability per command, or flag interactions that aren't covered in the main SKILL.md.

> **Defaults can be overridden by config.** The defaults below are the CLI's built-in flag defaults (`notebrain --help`). `~/.notebrain/config/config.toml` (or `--config`) can change effective defaults — notably `include-text`, `context-window`, `min-score`, `limit`, and `top-k` have config keys (`search & query settings` section). Example: with `include-text = true` and `context-window = 1` in config, every search result includes `text` and `context` **even when the flags are omitted**. If you need lean output, pass the flags explicitly (`--include-text=false`, `--context-window=0`) rather than assuming the built-in defaults.

## Command-Specific Flags

These flags are available only on the commands listed.

### `search`

| Flag                    | Purpose                                                                                                                                                                             | Default |
| ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--limit N`             | Maximum total results to return.                                                                                                                                                    | `10`    |
| `--top-k N`             | Maximum chunks to retain **per note**. Prevents one long note from dominating results.                                                                                              | `3`     |
| `--section "PATH"`      | Filter results to chunks under a specific heading path (e.g., `"Architecture > Components"`). **Exact equality** with the stored `heading_path` — partial or parent paths return 0 results silently. Copy the full `heading_path` value from a search result. | —       |
| `--tag "TagName"`       | Filter results to notes with this tag.                                                                                                                                              | —       |
| `--has-tasks`           | Only return chunks containing task lists (checkboxes).                                                                                                                              | off     |
| `--has-code`            | Only return chunks containing fenced code blocks.                                                                                                                                   | off     |
| `--with-pdf`            | Include PDF text extraction results in the search. Defaults to false (Markdown-only).                                                                                               | `false` |
| `--min-score F`         | Suppress results below this similarity score (0.0–1.0). Use to filter weak matches (e.g. `0.3` for meaningful hits, `0.5` for precision). Also available on `hidden` and `boosted`. | `0`     |
| `--group-by-note`       | Collapse results to one row per note: keeps the best-scoring chunk, drops the rest. When a note has multiple matching chunks, the surviving row gains `extra: "N matching chunks"`. Text/TSV/JSON all flow through this — handy for note-level result lists. | `false` |
| `--exclude-note "SLUG"` | Exclude notes from results. Accepts a note slug, title, or path, resolved automatically; repeat the flag or use comma-separated values. Unknown notes are skipped with a warning.   | —       |

> **Lexical fallback:** When semantic retrieval returns zero results — or every result is below `--min-score` — `search` automatically falls back to a token-based lexical scan over note titles, paths, tags, and text (case-insensitive, substring-style token matching, min token length 2). The header reads `Lexical Search (no semantic matches)` and rows are marked with `"lexical": true` in JSON (`score: 0`). This is why short queries like `Lecture` now return hits even when no semantic match clears the score bar. There is no lexical fallback for `boosted` or `hidden`.

> Note: To execute multi-query search with multi-hit boosting, pass multiple positional query arguments: `notebrain search "query1" "query2"`. When a title contains a literal comma, escape it with a backslash (`\,`) so it is not split into separate exclude values.

### `hidden`

| Flag               | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                         | Default |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--limit N`        | Maximum number of hidden connections to return.                                                                                                                                                                                                                                                                                                                                                                                                 | `10`    |
| `--deep`           | Analyze each chunk individually for granular section-level matches using stored vectors (no re-embedding required). **Requires the target note to have indexed chunks.** If resolution fails, the error names the resolved slug: `note "<slug>" has no indexed chunks ... run 'notebrain ingest' ...` — re-resolve the slug via `search` first if the note demonstrably has chunks. | `false` |
| `--top-k N`        | Chunks to evaluate per candidate note in `--deep` mode. Deprecated alias: `--candidate-chunks` (replaces `--top-k`).                                                                                                                                                                                                                                                                                                                             | `3`     |
| `--include-linked` | Include notes that are already linked directly/indirectly, while still excluding self-references.                                                                                                                                                                                                                                                                                                                                               | `false` |

### `connections`

| Flag       | Purpose                                                                                            | Default |
| ---------- | -------------------------------------------------------------------------------------------------- | ------- |
| `--hops N` | Breadth-first search traversal depth. Keep to 1–2 to avoid exponential blowup of returned results. | `2`     |

### `tags`

| Flag             | Purpose                                                                                                   | Default |
| ---------------- | --------------------------------------------------------------------------------------------------------- | ------- |
| `--list`         | List all indexed tags with note counts (query is ignored). Use `--limit N` to cap, `--limit 0` for all.   | `false` |
| `--shared`       | Treat the query as a note slug/title to find notes sharing its tags.                                      | `false` |
| `--children`     | Include child tags in hierarchical structure (e.g. searching 'kubernetes' also matches 'kubernetes/cka'). | `false` |
| `--min-shared N` | Minimum number of shared tags required to include a result (only applies when --shared is active).        | `1`     |
| `--limit N`      | Maximum number of results (0 = no limit for `--list`; searches default to 50). A `limit` key in config overrides this default — pass `--limit 0` explicitly for a full `--list` enumeration. | `0`     |

> Tag input is normalized: the `#` prefix is optional and matching is case-insensitive, so `tags "Kubernetes"` and `tags "#kubernetes"` are equivalent. Without `--children` the match is **exact** — `tags "k8s"` does not match the tag `kubernetes`. `--children` enables hierarchical prefix matching (`kubernetes` also matches `kubernetes/cka`). Tags are stored bare and lowercase; the JSON `tags` field is only emitted with `--show-tags`.
>
> When a tag search finds nothing, the CLI prints a "Did you mean: #go, #golang?" hint (Levenshtein-based, text output only — machine formats stay clean). Tag counts from `--list` are per-note (a note counts once even with many chunks).
>
> `tags --list` output shapes: text `#tag<TAB>(N notes)`, tsv `tag<TAB>count` with a header row, json `{"command":"tags --list","total":N,"tags":[{"tag":"...","count":N}]}` — all JSONPath-queryable.

### `boosted`

| Flag            | Purpose                                                                                 | Default |
| --------------- | --------------------------------------------------------------------------------------- | ------- |
| `--seed STRING` | **Required.** Seed note (slug, title, or path) whose graph neighbors get a score boost. | —       |
| `--limit N`     | Maximum number of results.                                                              | `10`    |
| `--boost F`     | Score multiplier for graph-connected results (e.g., `1.5` = 50% boost over base score). | `1.5`   |
| `--with-pdf`    | Include PDF text extraction results in the search. Defaults to false (Markdown-only).   | `false` |

### `get`

| Flag        | Purpose                                                                                                  | Default |
| ----------- | -------------------------------------------------------------------------------------------------------- | ------- |
| `--meta`    | Header only: title, path, tags, and total chunk count — no note text. Cheap way to read tags/slug/path. | `false` |
| `--head N`  | Return only the first N chunks of text. `Chunks` still reports the full total.                            | `0`     |

Takes a single positional argument: `<slug>` (note slug, title, or file path — auto-resolved). Without flags, `get` returns the full reconstructed note; in text format, the header block prints a `Tags:` line (rendered as `#`-chips) — a lightweight way to read a note's tags without JSON. `--meta`/`--head` are mutually independent modes; `--head 0` means full note.

## Global Flags (Available on Subcommands)

These flags work on `search`, `backlinks`, `connections`, `hidden`, `tags`, `boosted`, `get`, and `stats`.

### Output Format & Extraction

| Flag               | Purpose                                                                                                                                                                                                                                                                                                                                                   | Default |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--format FORMAT`  | Output format: `json` (structured envelope), `tsv` (tab-separated, no key names), `text` (standard text).                                                                                                                                                                                                                                                 | `text`  |
| `--show-file-path` | Include the `file_path` field in output (use `--show-file-path=false` to hide).                                                                                                                                                                                                                                                                           | `true`  |
| `--jsonpath PATH`  | Extract specific JSON elements with JSONPath (e.g., `"$.results[*].note_slug"`). Drops the JSON envelope entirely. Dialect and valid/missing-path behavior: see the note below the table. For multi-field extraction use `--format tsv` or two `--jsonpath` calls. | —       |
| `--include-text`   | Include the matched markdown text chunk in results. Omit during initial structure-mapping to save tokens.                                                                                                                                                                                                                                                 | off     |

> **`--jsonpath` dialect** (PaesslerAG/jsonpath v0.1.1, verified): dotted paths (`.key`), array wildcards (`[*]`), numeric indices (`[0]`), slices (`[0:2]`), and recursive descent (`$..key`) all work; bracket-quoted keys (`$['key']`), filter expressions (`[?(...)]`), and jq-style pipes do NOT. A wildcard result is a JSON array; a missing key is an error (exit 1). Path shorthand is normalized: `results[*].note_slug`, `$.results[*].note_slug`, and `{.results[*].note_slug}` (jq-style braces) are equivalent.

### Search & Display Filtering

| Flag                 | Purpose                                                                                                                                                                                                                                                        | Default |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--context-window N` | Fetch ±N adjacent chunks around each match into the `context` field. Use for lightweight surrounding context across results.                                                                                                                                   | `0`     |
| `--min-score F`      | Suppress results below this similarity score (0.0–1.0).                                                                                                                                                                                                        | `0`     |
| `--show-tags`        | Include tag names in output. In JSON they are bare and lowercase (e.g., `"kubernetes"`, no `#`); in CLI text they render as `#`-chips (`#kubernetes`). The field/line is omitted entirely for notes without tags, so its absence does not mean the flag is broken. | `false` |
| `--skip-phantom`     | Exclude uncreated notes (phantom wikilinks without a `.md` file on disk) from results.                                                                                                                                                                         | `true`  |

### Environment & Diagnostics

| Flag                  | Purpose                                        | Default                           |
| --------------------- | ---------------------------------------------- | --------------------------------- |
| `--chroma-path PATH`  | Path to ChromaDB persistent storage directory. | `~/.notebrain/chroma`             |
| `--vault-path PATH`   | Path to the Obsidian vault directory.          | (from config)                     |
| `--vault-name STRING` | Vault display name for Obsidian URI links.     | basename of `--vault-path`        |
| `--config PATH`       | Path to config file.                           | `~/.notebrain/config/config.toml` |
| `--debug`             | Enable debug-level logging to stderr.          | `false`                           |

> Hyperlink suppression: To disable OSC 8 terminal hyperlinks, set environment variable `NO_HYPERLINKS=1`.
