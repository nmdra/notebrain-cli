# NoteBrain Scenario Guide (Worked Examples)

Quick reference: the major scenarios with the proven command sequence. Pair with [flags.md](flags.md) (flag details) and [schema.md](schema.md) (output fields).

> Outputs are illustrative — scores, counts, tags, and slugs vary per vault. Replace `<slug>` with real values from a prior `search`/`tags` call.

## Scenarios

| #   | Scenario                 | Command(s)                                                                                                                                 |
| --- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| 1   | Pre-flight               | `notebrain stats --format=json` — if `chunks: 0`, the vault is not indexed; tell the user to run `notebrain ingest`                        |
| 2   | Slug discovery           | `notebrain search "<topic>" --limit 3 --jsonpath="$.results[*].note_slug"`                                                                 |
| 3   | Tag discovery            | `notebrain tags --list --format tsv` (full enumeration); or `search "<topic>" --limit 1 --show-tags --jsonpath="$.results[0].tags"`; fallback: `get "<slug>" --format text` (`Tags:` line)   |
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
| 15  | Context vs full `get`    | context: `--context-window 1 --include-text`; full note only on explicit demand: `get "<slug>"`                                            |
| 16  | Stale-index recovery     | a slug that 404s mid-conversation → re-resolve: `search "<title>" --limit 3 --jsonpath="$.results[*].note_slug"`                           |
| 17  | Reference inventory      | `notebrain refs "<slug>" --format json` (all kinds); kind filters: `--only-images` / `--only-pdf` / `--only-other` / `--only-external-links`                    |
| 18  | Broken-link audit        | `notebrain refs "<slug>" --include-missing --format tsv` → rows with `missing` = `true` are broken; omit `--include-missing` to see only existing files |

## Semantics (verified)

- **Tag matching**: exact match, case-insensitive, `#`-optional — `tags "Kubernetes"` ≡ `tags "#kubernetes"`. Substrings do **not** match (`tags "k8s"` finds nothing for `kubernetes`). `--children` = hierarchical prefix (`kubernetes` also matches `kubernetes/cka`).
- **Tags in JSON**: present only with `--show-tags`; bare and lowercase (`["kubernetes"]`); omitted for untagged notes. Text output renders them as `#`-chips.
- **`--section` is exact-match**: it compares against the stored `heading_path` string verbatim. Partial or parent paths return 0 results silently — copy the full `heading_path` from a search result.
- **`--jsonpath`**: dotted paths, `[*]`, and `[0]` only — no jq-style pipe expressions, filters, or object construction. Multi-field extraction → `--format tsv` or two `--jsonpath` calls.
- **Config overrides defaults**: `~/.notebrain/config/config.toml` can enable `include-text`/`context-window` (and set `min-score`/`limit`/`top-k`) — output then carries `text`/`context` even without flags. Pass `--include-text=false`/`--context-window=0` explicitly for lean output.
- **`refs` reads the file, not the index**: results come from a fresh parse of the note on disk — never stale, but only reflect what the current file contains. Order is first occurrence in the note; rows dedupe by resolved path (or exact URL).

## Pitfalls (verified)

- **Tag discovery**: never guess tag spelling — discover via scenario 3 (vault tags drift: you remember `K8S`, the vault stores `kubernetes`).
- **Slug discipline**: always pass the exact `note_slug` (never bare titles) to `get`/`backlinks`/`connections`/`hidden`/`boosted`. Near-duplicate titles can silently resolve to a phantom slug; `hidden --deep` then fails with `note "<slug>" has no indexed chunks ... run 'notebrain ingest' first` (misleading hint — re-resolve via `search`).
- **Duplicate rows**: one note can span multiple chunk rows — normal. For distinct notes use `--top-k 1`, or dedupe: `--jsonpath="$.results[*].note_slug" | sort -u` (piping `notebrain` stdout is fine).
- **Weak matches**: add `--min-score 0.3` (or `0.5` for precision); results below ~0.30 are noise. Note: config may already set a `min-score` floor, so low-score results can be absent by design.
- **`get`**: `--meta` (header only: title, path, tags, chunk count) or `--head N` (first N chunks, `Chunks` still shows the total) cover most needs for cheap reads — reach for the full note only on demand. For metadata see also scenarios 3/14.
- **Stale index**: scheduled re-ingest can invalidate cached slugs mid-conversation; re-verify via `search` before `--deep`/`backlinks` after any 404.
- **`refs` scope**: attachments (image/pdf/other) and external http(s) links only — links to other notes never appear (use `backlinks`/`connections`). So `refs --include-missing` catches broken attachments, not broken `[[wikilinks]]` to notes. `refs` is markdown-notes only; PDF extractions error out.

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
