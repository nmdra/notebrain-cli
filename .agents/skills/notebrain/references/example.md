# NoteBrain Scenario Guide (Worked Examples)

Quick reference: the major scenarios with the proven command sequence. Pair with [flags.md](flags.md) (flag details) and [schema.md](schema.md) (output fields).

> Outputs are illustrative — scores, counts, tags, and slugs vary per vault. Replace `<slug>` with real values from a prior `search`/`tags` call.

## Scenarios

| #   | Scenario                 | Command(s)                                                                                                                                 |
| --- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | Pre-flight               | `notebrain stats --format=json` — if `chunks: 0`, the vault is not indexed; tell the user to run `notebrain ingest`                        |
| 2   | Slug discovery           | `notebrain search "<topic>" --limit 3 --jsonpath="$.results[*].note_slug"`                                                                 |
| 3   | Tag discovery            | `notebrain tags --list --format tsv` (full enumeration); or `search "<topic>" --limit 1 --show-tags --jsonpath="$.results[0].tags"`; fallback: `get "<slug>" --meta --format json --jsonpath="$.note.tags"`   |
| 4   | Tag query                | `notebrain tags "kubernetes" --format json --show-tags`; children: `tags "kubernetes" --children`; shared: `tags "<slug>" --shared --min-shared 1` |
| 5   | List all notes tagged X  | `notebrain tags "X" --children --limit 50 --format tsv`                                                                                    |
| 6   | Semantic search          | `search "<q>" --format=json --include-text --limit 3`; escalate: `--top-k 2 --context-window 1`; stop when top score ≥ 0.75                |
| 7   | Multi-topic comparison   | `notebrain search "redis pubsub" "kafka brokers" --limit 5 --top-k 2 --format json`                                                        |
| 8   | Filtered search          | add `--tag "kubernetes"`, `--section "Architecture > Components"`, `--has-tasks`, `--has-code`, `--exclude-notes "<slug>"`, `--min-score 0.3` |
| 9   | Zero-result handling     | short common words now fall back to a lexical token scan (`"lexical": true`, `score: 0`); if still nothing → longer descriptive phrase or `tags` query; never grep the vault          |
| 10  | Backlinks                | `notebrain backlinks "<slug>" --format json --limit 10`                                                                                    |
| 11  | Connections              | `notebrain connections "<slug>" --hops 2 --format tsv`                                                                                     |
| 12  | Hidden connections       | `notebrain hidden "<slug>" --limit 5 --format json`; section-level: `--deep`                                                               |
| 13  | Boosted search           | `notebrain boosted --seed="<slug>" "<query>" --limit 5 --format json`                                                                      |
| 14  | Metadata-only extraction | `--jsonpath`, `--format tsv`, `--show-file-path=false` (cuts ~40–50% of tokens)                                                            |
| 15  | Context vs full `get`    | context: `--context-window 1 --include-text`; full note in `text` only when the note is for the user to read — otherwise stay on `json`/`--jsonpath` |
| 16  | Stale-index recovery     | a slug that 404s mid-conversation → re-resolve: `search "<title>" --limit 3 --jsonpath="$.results[*].note_slug"`                           |
| 17  | Reference inventory      | `notebrain refs "<slug>" --format json` (all kinds); kind filters: `--only-images` / `--only-pdf` / `--only-other` / `--only-external-links`                    |
| 18  | Broken-link audit        | `notebrain refs "<slug>" --include-missing --format tsv` → rows with `missing` = `true` are broken; omit `--include-missing` to see only existing files |

## Semantics (verified)

- **`--section` is exact-match**: it compares against the stored `heading_path` string verbatim. Partial or parent paths return 0 results silently — copy the full `heading_path` from a search result.
- **`refs` reads the file, not the index**: results reflect the current file contents, never stale index state. Scope and ordering: [SKILL.md](../SKILL.md).
- **Rules shared with the main skill**: format policy and the config-overrides trap → [SKILL.md](../SKILL.md); tag matching, `--jsonpath` dialect, and weak-match floors → [flags.md](flags.md); tags-in-JSON and output shapes → [schema.md](schema.md). Each meaning lives in exactly one place — read it there.

## Pitfalls (verified)

- **Tag discovery**: never guess tag spelling — discover via scenario 3 (vault tags drift: you remember `K8S`, the vault stores `kubernetes`).
- **Duplicate rows**: one note can span multiple chunk rows — normal. For distinct notes use `--top-k 1`, or dedupe: `--jsonpath="$.results[*].note_slug" | sort -u` (piping `notebrain` stdout is fine).
- **Slug discipline & staleness**: pass the exact `note_slug` from a prior `search`/`tags` — never a bare title; re-resolve via `search` when a slug 404s (scenario 16). Details: [SKILL.md](../SKILL.md).
- **Command-specific gotchas**: `get` `--meta`/`--head` modes ([flags.md](flags.md)); `refs` covers attachments and external links only, never note-to-note links ([SKILL.md](../SKILL.md)); weak matches → `--min-score` precision floors ([flags.md](flags.md)).

## Phrase → Scenario Map

| User phrase                                          | Scenario |
| ---------------------------------------------------- | -------- |
| "what do I know about X" / "summarize my notes on W" | 6 → 15   |
| "find notes related to Y"                            | 6 → 12   |
| "what connects to Z"                                 | 11       |
| "what links to this note"                            | 10       |
| "list all notes tagged X"                            | 5        |
| "notes tagged something like X" / "what tags exist"  | 3 → 4    |
| "notes about X tagged Y"                             | 8        |
| "unlinked / hidden concepts near Y"                  | 12       |
| "concepts about X around note Y"                     | 13       |
| "everything on topic X"                              | 5 or 6   |
| "what images / attachments does this note use"       | 17       |
| "are any links / attachments broken"                 | 18       |
| "why did that search return nothing"                 | 9        |
