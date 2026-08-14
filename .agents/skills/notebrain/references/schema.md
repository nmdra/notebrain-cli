# NoteBrain Output Schema & Format Guide

This reference documents the structure of NoteBrain's output in each format. Read this when you need to understand JSON field meanings, parse `tsv` columns, or write `--jsonpath` expressions.

## Output Formats (`--format`)

The CLI's built-in default is `text`, but the skill mandates machine formats for agents. `text` is reserved for human-facing display only (showing a note body via `get`, or when the user asks to see raw CLI output).

| Format | Agent Use |
| ------ | --------- |
| `json` | Default for agents. Structured envelope with `results` array. Pair with `--jsonpath` for extraction. |
| `tsv`  | Token-optimized for wide, shallow scans — no repeating key names. Use for `tags --list`, `backlinks`, `connections`, `refs` audits, `--group-by-note` lists. |
| `text` | Human reading only. Never for parsing or summarizing — it carries headers, `#`-chips, and "Did you mean" hints. |

### Per-command preference

| Command | Format | Why |
| ------- | ------ | --- |
| `search`, `hidden`, `boosted` | `json` | Nested fields: `text`, `context`, `heading_path`, `matched_queries`, `tags` (with `--show-tags`). |
| `backlinks`, `connections` | `tsv` | Flat rows, few columns; `tsv` halves token cost vs repeated JSON keys. |
| `tags --list` | `tsv` | `tag<TAB>count` pairs. |
| `tags` (search/shared/children) | `json` | `tags` arrays need `--show-tags`; `tsv` also works. |
| `refs` | `tsv` (audit) / `json` (inventory) | `tsv` exposes the `missing` column for broken-link audits; `json` keeps `path`/`kind` structured. |
| `get --meta` | `json` + `--jsonpath` | `--jsonpath="$.note.tags"` etc.; no body fetched. |
| `get` (full note) | `text` — only for human display | The reconstructed note is the artifact a human reads; the agent should not need the full body for parsing. |
| `stats` | `json` | `--jsonpath="$.chunks"` for the preflight check. |

## JSON Envelope Structure

When `--format=json` is used, the response has this top-level shape:

```json
{
  "command": "search",
  "query": "...",
  "total": N,
  "results": [...]
}
```

### Result Fields

Each item in the `results` array may contain:

| Field             | Present When                                       | Description                                                                                                           |
| ----------------- | -------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `note_slug`       | Always                                             | URL-safe unique identifier derived from the file path. Used as input for graph/get commands.                          |
| `title`           | Always                                             | Note title extracted from frontmatter or filename.                                                                    |
| `file_path`       | Always (unless `--show-file-path=false`)           | Relative file path within the vault. Included by default; hide with `--show-file-path=false` to save tokens.          |
| `file_type`       | Always                                             | Source format: `"md"` for markdown notes, `"pdf"` for PDF extractions (returned only with `--with-pdf`). Not affected by `--show-file-path=false`. |
| `score`           | Always                                             | Similarity score (0–1) for semantic search; hop count for graph connections. JSON rounds to 4 decimals; TSV prints 6. |
| `chunk_index`     | search, hidden, boosted — **unless 0**             | Which chunk of the note matched the query (0-indexed). Omitted when the match landed on chunk 0 (Go `omitempty`), so absence is normal. |
| `tags`            | When `--show-tags` is passed and the note has tags | Array of tag strings, bare and lowercase (e.g., `["kubernetes"]` — no `#` prefix). Omitted entirely for untagged notes. |
| `heading_path`    | When chunk is under a heading                      | Breadcrumb path hierarchy (e.g., `"Section > Subsection"`).                                                           |
| `text`            | When `--include-text` is passed (or enabled via config) | The matched chunk's full markdown text, preserving code blocks and formatting.                                  |
| `context`         | When `--context-window N` > 0 (or enabled via config) | Array of ±N adjacent chunk texts around the match (excluding the matched chunk itself).                           |
| `extra`           | backlinks, connections, boosted, search (`--group-by-note`) | Command-specific metadata: backlinks → link display text; connections → `"N hop(s)"`; boosted → `"graph-boosted"` (only on graph-linked results); search with `--group-by-note` → `"N matching chunks"` when the note matched multiple chunks. Absent for `tags`, `hidden`, and non-boosted `boosted` rows. |
| `lexical`         | search lexical fallback only                                | `true` when the row came from the token-based lexical fallback (no semantic match cleared the bar). Such rows carry `"score": 0`, no `chunk_index`, no `text`, and rank below any semantic rows. Absent on semantic rows. |
| `is_phantom`      | Only when the note **is** a phantom                | `true` if the note is an uncreated phantom link without a `.md` file on disk. Absent for real notes — do not treat absence as a broken flag. Visible with `--skip-phantom=false`. |
| `matched_queries` | hidden `--deep`, multi-query                       | Array of queries or section headings (`§ <HeadingPath>`) that matched this candidate.                                 |

## Example Outputs

### Semantic Search (with text and context)

`notebrain search "event driven architecture" --format=json --include-text --context-window 1 --show-tags`

```json
{
  "total": 1,
  "results": [
    {
      "note_slug": "architecture/event-driven-systems",
      "title": "Event Driven Systems",
      "score": 0.852,
      "chunk_index": 2,
      "tags": ["architecture", "distributedsystems"],
      "heading_path": "Overview > Message Brokers",
      "text": "Message brokers decouple producers from consumers...",
      "context": [
        "Producers publish events without knowing who consumes them...",
        "Consumers process events at their own pace..."
      ],
      "matched_queries": ["message brokers"]
    }
  ]
}
```

### Graph & Structure Mapping

`notebrain connections "architecture/event-driven-systems" --hops 1 --format=json --show-tags`

```json
{
  "total": 1,
  "results": [
    {
      "note_slug": "database/redis-streams",
      "title": "Redis Streams",
      "score": 1.0,
      "tags": ["database", "redis"],
      "extra": "1 hop(s)"
    }
  ]
}
```

### Direct Tag Search

`notebrain tags "architecture" --children --format=json --show-tags`

```json
{
  "total": 2,
  "results": [
    {
      "note_slug": "architecture/event-driven-systems",
      "title": "Event Driven Systems",
      "score": 1.0,
      "tags": ["architecture", "distributedsystems"]
    },
    {
      "note_slug": "architecture/microservices/intro",
      "title": "Microservices Introduction",
      "score": 1.0,
      "tags": ["architecture/microservices", "go"]
    }
  ]
}
```

Tag matching and normalization semantics: see `tags` in [flags.md](flags.md).

### Full Note Retrieval (`get`)

`get` does **not** use the `results` envelope — it returns a `note` object wrapping the full reconstructed note:

`notebrain get "architecture/event-driven-systems" --format=json`

```json
{
  "command": "get",
  "query": "architecture/event-driven-systems",
  "note": {
    "note_slug": "architecture/event-driven-systems",
    "title": "Event Driven Systems",
    "file_path": "Architecture/Event Driven Systems.md",
    "tags": ["architecture", "distributedsystems"],
    "text": "## Overview\n\nMessage brokers decouple producers...",
    "chunks": 12
  }
}
```

| Field       | Description                                              |
| ----------- | -------------------------------------------------------- |
| `note_slug` | URL-safe unique identifier.                              |
| `title`     | Note title.                                              |
| `file_path` | Relative path within the vault.                          |
| `tags`      | Bare lowercase tags (`#`-optional; see Tag Matching Rules). |
| `text`      | Full reconstructed note, section headers (`##`) prepended by the CLI. |
| `chunks`    | Total indexed chunk count for the note.                  |

The `note` object shape is the same for the `get` modes: default returns the full text, `--meta` returns the header with `text` empty (and `chunks` still the total), and `--head N` returns the first N chunks while `chunks` still reports the full total. For metadata-only lookups use `get "<slug>" --meta --format json` (extract with `--jsonpath`, e.g. `$.note.tags`). The full note in `text` format is for human reading — show it to the user, don't parse it.

### `refs`

`refs` uses its own envelope — a `refs` array (not `results`):

`notebrain refs "architecture/event-driven-systems" --format=json`

```json
{
  "command": "refs",
  "note_slug": "architecture/event-driven-systems",
  "title": "Event Driven Systems",
  "total": 3,
  "refs": [
    {
      "path": "/home/user/vault/Attachments/eda-diagram.png",
      "relative_path": "Attachments/eda-diagram.png",
      "kind": "image",
      "missing": false
    },
    {
      "path": "https://martinfowler.com/articles/eda.html",
      "kind": "external-links",
      "missing": false
    }
  ]
}
```

| Field           | Description                                                                                                              |
| --------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `note_slug`     | URL-safe unique identifier of the note queried.                                                                          |
| `title`         | Note title.                                                                                                              |
| `total`         | Number of rows in `refs`.                                                                                                |
| `refs[].path`   | Absolute vault path for attachments; the full URL for external links.                                                    |
| `refs[].relative_path` | Vault-relative path (`/`-separated). Omitted for external links.                                                  |
| `refs[].kind`   | `image`, `pdf`, `other`, or `external-links`.                                                                            |
| `refs[].missing`| `true` when the file is not on disk (only visible with `--include-missing`). External links always carry `false` — they are never missing and never contacted over the network. |

Rows are deduped by resolved path (or exact URL) in first-occurrence order. No kind flags = every kind.

TSV shape — header `path<TAB>kind<TAB>missing<TAB>relative_path`; the `missing` cell shows `false` for external links:

<!-- markdownlint-disable MD010 -->
```text
path	kind	missing	relative_path
/home/user/vault/Attachments/eda-diagram.png	image	false	Attachments/eda-diagram.png
/home/user/vault/broken.png	image	true	broken.png
https://martinfowler.com/articles/eda.html	external-links	false	
```
<!-- markdownlint-enable MD010 -->

Text format prints one row per reference: `[kind] path (missing)` — e.g. `[image] /home/user/vault/broken.png (missing)`. Empty result prints `No references found`.

### TSV Format

`notebrain backlinks "architecture/event-driven-systems" --format=tsv`

<!-- markdownlint-disable MD010 -->
```text
slug	title	file_path	score	tags	extra	heading_path	text
database/redis-streams	Redis Streams	Database/Redis Streams.md	1.000000	database, redis			
messaging/kafka-intro	Kafka Introduction	Messaging/Kafka Introduction.md	1.000000	messaging			
```
<!-- markdownlint-enable MD010 -->

First line is the header row (always emitted). Columns are tab-separated. Tags are comma-joined into a single cell, bare and lowercase (no `#`). Cells for absent fields (`extra`, `heading_path`, `text`) are empty tabs. Note: scores print with **6** decimals in TSV (`1.000000`), vs 4 in JSON (`1.0000`).

### Stats

`notebrain stats --format=json`

```json
{
  "command": "stats",
  "chunks": 8993,
  "links": 1255,
  "notes": 783
}
```

Use this for pre-flight checks — if `chunks` is `0`, the vault hasn't been indexed yet.

## Extracting Fields via `--jsonpath`

Use `--jsonpath` to extract exactly the fields you need without loading the full JSON envelope. Dialect rules (dotted paths, `[*]`, `[0]`; no filters/pipes/bracket keys) and shorthand normalization live in [flags.md](flags.md) — read those before writing expressions.

```bash
# Note slugs only (newline-separated)
notebrain search "event driven architecture" --limit 5 --jsonpath="$.results[*].note_slug"

# Full text of the top matching chunk
notebrain search "jwt authentication" --limit 1 --include-text --jsonpath="$.results[0].text"

# Just the chunk count from stats
notebrain stats --format=json --jsonpath="$.chunks"

# All scores to assess result quality
notebrain search "kubernetes" --limit 5 --jsonpath="$.results[*].score"
```

`--jsonpath` outputs raw values (no JSON envelope), one per line. When extracting a single scalar (e.g., `$.results[0].text`), the output is the bare value with no surrounding quotes or brackets.
