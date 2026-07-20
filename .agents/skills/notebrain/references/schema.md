# NoteBrain Output Schema & Format Guide

This reference documents the structure of NoteBrain's output in each format. Read this when you need to understand JSON field meanings, parse `tsv` columns, or write `--jsonpath` expressions.

## Output Formats (`--format`)

| Format   | When to Use                                                                                             |
| -------- | ------------------------------------------------------------------------------------------------------- |
| `json`   | Default for agents. Structured envelope with `results` array. Pair with `--compact` to cut token bloat. |
| `tsv`    | Token-optimized for scan-only steps — no repeating key names. Good for backlinks, connections, tags.    |
| `ndjson` | One JSON object per line (no envelope). Useful for streaming or line-by-line parsing.                   |
| `text`   | Rich TUI output for human reading. Not useful for agents — avoid in automated workflows.                |

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

With `--compact`, the `command` and `query` envelope fields are stripped, leaving only `total` and `results`.

### Result Fields

Each item in the `results` array may contain:

| Field             | Present When                    | Description                                                                                               |
| ----------------- | ------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `note_slug`       | Always                          | URL-safe unique identifier derived from the file path. Used as input for graph/get commands.              |
| `title`           | Always                          | Note title extracted from frontmatter or filename.                                                        |
| `file_path`       | Unless `--compact`              | Relative file path within the vault. Omitted when `--compact` is active.                                  |
| `score`           | Always                          | Similarity score (0–1) for semantic search; hop count for graph connections. Rounded to 4 decimal places. |
| `chunk_index`     | search, hidden, boosted         | Which chunk of the note matched the query (0-indexed).                                                    |
| `tags`            | When note has tags              | Array of tag strings (e.g., `["#Architecture", "#Go"]`).                                                  |
| `heading_path`    | When chunk is under a heading   | Breadcrumb path hierarchy (e.g., `"Section > Subsection"`).                                               |
| `text`            | When `--include-text` is passed | The matched chunk's full markdown text, preserving code blocks and formatting.                            |
| `context`         | When `--context-window N` > 0   | Array of ±N adjacent chunk texts around the match (excluding the matched chunk itself).                   |
| `extra`           | connections, tags, boosted      | Command-specific metadata (e.g., `"2 hop(s)"`, `"graph-boosted"`).                                        |
| `is_phantom`      | When `--skip-phantom=false`     | `true` if the note is an uncreated phantom link without a `.md` file on disk.                             |
| `matched_queries` | hidden `--deep`, multi-query    | Array of queries or section headings (`§ <HeadingPath>`) that matched this candidate.                     |

## Example Outputs

### Semantic Search (compact, with text and context)

`notebrain search "event driven architecture" --format=json --compact --include-text --context-window 1`

```json
{
  "total": 1,
  "results": [
    {
      "note_slug": "architecture/event-driven-systems",
      "title": "Event Driven Systems",
      "score": 0.852,
      "chunk_index": 2,
      "tags": ["#Architecture", "#DistributedSystems"],
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

### Graph & Structure Mapping (compact)

`notebrain connections "architecture/event-driven-systems" --hops 1 --format=json --compact`

```json
{
  "total": 1,
  "results": [
    {
      "note_slug": "database/redis-streams",
      "title": "Redis Streams",
      "score": 1.0,
      "tags": ["#Database", "#Redis"],
      "extra": "1 hop(s)"
    }
  ]
}
```

### Direct Tag Search (compact)

`notebrain tags "#Architecture" --children --format=json --compact`

```json
{
  "total": 2,
  "results": [
    {
      "note_slug": "architecture/event-driven-systems",
      "title": "Event Driven Systems",
      "score": 1.0,
      "tags": ["#Architecture", "#DistributedSystems"]
    },
    {
      "note_slug": "architecture/microservices/intro",
      "title": "Microservices Introduction",
      "score": 1.0,
      "tags": ["#Architecture/Microservices", "#Go"]
    }
  ]
}
```

### TSV Format

`notebrain backlinks "architecture/event-driven-systems" --format=tsv`

```
note_slug	title	file_path	score	tags
database/redis-streams	Redis Streams	Database/Redis Streams.md	1.0000	#Database, #Redis
messaging/kafka-intro	Kafka Introduction	Messaging/Kafka Introduction.md	1.0000	#Messaging
```

First line is the header row. Columns are tab-separated. Tags are comma-joined into a single cell.

### Stats

`notebrain stats --format=json --compact`

```json
{
  "chunks": 8993,
  "links": 1255,
  "notes": 783
}
```

Use this for pre-flight checks — if `chunks` is `0`, the vault hasn't been indexed yet.

## Extracting Fields via `--jsonpath`

Use `--jsonpath` to extract exactly the fields you need without loading the full JSON envelope into context:

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
