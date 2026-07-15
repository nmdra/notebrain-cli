# NoteBrain Output Schema & Format Guide

When executing NoteBrain queries non-interactively, selecting the right format and understanding the response structure saves tokens and prevents parsing errors.

## Output Formats (`--format`)

- **`json` (Default Recommendation)**: Returns a structured JSON envelope containing a `results` array and metadata. Best when using `--jsonpath` or when inspecting complex multi-field objects.
- **`tsv` (Token-Optimized for Scan-Only Steps)**: Returns tab-separated values without repeating JSON key names (`note_slug`, `title`, `file_path`, `score`, `tags`) on every row. Highly recommended when scanning lists of notes (e.g., mapping backlinks or graph connections without `--include-text`).

## JSON Envelope Field Specification

When `--format=json` is used, each item in the `results` array conforms to the following schema:

| Field             | Present When                    | Description                                                                                  |
| ----------------- | ------------------------------- | -------------------------------------------------------------------------------------------- |
| `note_slug`       | Always                          | URL-safe unique identifier derived from the file path. Used as input for graph/get commands. |
| `title`           | Always                          | Note title extracted from frontmatter or filename.                                           |
| `file_path`       | Always (unless `--compact`)     | Relative file path within the Obsidian vault. Omitted when `--compact` is active.           |
| `score`           | Always                          | Similarity score (0–1, rounded to 4 decimal places) for semantic search; hop count for graph connections. |
| `chunk_index`     | Search, hidden, boosted         | Which chunk of the note matched the query (0-indexed).                                       |
| `tags`            | When note has tags              | Array of tag strings associated with the note.                                               |
| `heading_path`    | When chunk is under a heading   | Breadcrumb path hierarchy like `"Section > Subsection"`.                                     |
| `text`            | When `--include-text` is passed | The matched chunk's full markdown text, with code blocks and formatting preserved.           |
| `context`         | When `--context-window N` > 0   | Array of ±N adjacent chunk texts around the match (specifically excluding the matched chunk `text` itself to prevent token redundancy). |
| `extra`           | Connections, tags, boosted      | Command-specific metadata string (e.g., `"2 hop(s)"`, `"graph-boosted"`).                    |
| `is_phantom`      | When `--skip-phantom=false`     | Boolean (`true`) if the note is an uncreated phantom link without a `.md` file on disk.      |
| `matched_queries` | When results match queries      | Array of queries or initial note section headings (`§ <HeadingPath>`) that matched this candidate note (`hidden --deep` attribution). |

## Example JSON Output Schemas

### 1. Semantic Search (`search`, `hidden`, `boosted`)

When `--format=json`, `--include-text`, and `--context-window 1` are passed:

```json
{
  "command": "search",
  "query": "event driven architecture",
  "total": 1,
  "results": [
    {
      "note_slug": "architecture/event-driven-systems",
      "title": "Event Driven Systems",
      "file_path": "Architecture/Event Driven Systems.md",
      "score": 0.8520,
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

### 2. Graph & Structure Mapping (`connections`, `backlinks`, `tags`)

When `--format=json` is passed without text:

```json
{
  "results": [
    {
      "note_slug": "database/redis-streams",
      "title": "Redis Streams",
      "file_path": "Database/Redis Streams.md",
      "score": 1.0,
      "tags": ["#Database", "#Redis"],
      "extra": "1 hop(s)",
      "is_phantom": false
    }
  ],
  "total": 1
}
```

## Extracting Fields via `--jsonpath`

To avoid loading full JSON envelopes into context, append `--jsonpath` to extract exact scalar values or arrays directly:

```bash
# Extract only note slugs as a clean newline-separated list
notebrain search "event driven architecture" --limit 5 --jsonpath="$.results[*].note_slug"

# Extract the full text of the top matching chunk
notebrain search "jwt authentication" --limit 1 --include-text --jsonpath="$.results[0].text"

# Extract database stats
notebrain stats --format=json --jsonpath="$.chunks"
```
