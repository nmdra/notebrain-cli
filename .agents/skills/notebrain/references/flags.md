# NoteBrain CLI Flag Reference

This reference documents all available command flags for fine-tuning search, graph traversal, filtering, and formatting.

## Search Flags (`notebrain search`)

| Flag                 | Purpose                                                                                                                                            | Default |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--top-k N`          | Maximum chunks to retain **per note**. Prevents one long note from dominating results while preserving depth across diverse notes.                 | `3`     |
| `--context-window N` | Fetches ±N adjacent chunks around each match into `context`. Use for lightweight multi-result context; use `get` only when you need the full note. | `0`     |
| `--split`            | Split query string by delimiters (comma, pipe, semicolon) or execute multi-positional queries. Activates multi-hit score boosting.                 | off     |
| `--split-by "CHARS"` | Delimiter characters used to tokenize query strings when `--split` is active.                                                                      | `",|;"`  |
| `--has-tasks`        | Only return chunks that contain task lists (checkboxes).                                                                                           | off     |
| `--has-code`         | Only return chunks that contain fenced code blocks.                                                                                                | off     |
| `--section`          | Filter results to chunks under a specific heading path (e.g., `"Architecture > Components"`).                                                      | —       |
| `--limit N`          | Maximum total results to return.                                                                                                                   | `10`    |
| `--tag "TagName"`    | Filter or search by tag name.                                                                                                                      | —       |
| `--min-score F`      | Suppress results below this similarity score (0–1).                                                                                                | `0.4`   |
| `--hide-tags`        | Hide tag names (`#Tag/Subtag`) in search and graph outputs.                                                                                        | `true`  |

## Graph & Link Filtering Flags (`backlinks`, `connections`, `hidden`, `tags`)

| Flag                 | Purpose                                                                                               | Default |
| -------------------- | ----------------------------------------------------------------------------------------------------- | ------- |
| `--skip-attachments` | Exclude attachment and image links (e.g., `.webp`, `.png`, `.canvas`) from graph edges and backlinks. | `true`  |
| `--skip-phantom`     | Exclude uncreated notes (phantom wikilinks without a markdown file on disk) from results.             | `true`  |
| `--hops N`           | Breadth-first search traversal depth for `connections`. Keep to 1 or 2 to avoid exponential blowup.   | `1`     |
| `--min-shared N`     | Minimum number of shared tags required when running `tags`.                                           | `1`     |

## Global Formatting & Extraction Flags

| Flag                 | Purpose                                                                                                                                            | Default |
| -------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--format FORMAT`    | Output format: `json` (structured array), `ndjson` (streamed objects), `tsv` (tab-separated values), or `text` (human-readable TUI/plain text).    | `text`  |
| `--jsonpath PATH`    | Extract specific JSON elements using JSONPath syntax (e.g., `"$.results[*].note_slug"`). Eliminates JSON envelope overhead and avoids needing `jq`.| —       |
| `--include-text`     | Include the matched markdown text chunk in the output. Omit during initial structure-mapping to save tokens.                                       | off     |
| `--use-editor`       | Enable external editor (`$EDITOR`) integration as default open type.                                                                               | `false` |
