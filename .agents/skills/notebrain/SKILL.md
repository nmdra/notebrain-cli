---
name: notebrain-assistant
description: Search and explore an Obsidian vault through the NoteBrain CLI (semantic search, tags, backlinks, connections, hidden links, boosted retrieval). Use it whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections or unlinked notes, or asks exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", "summarize my notes on W", "what does this note reference or embed", or "are any links broken" — even when they never say NoteBrain, vector search, or ChromaDB.
license: MIT
compatibility: Requires the `notebrain` binary on PATH.
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain Assistant Skill

NoteBrain indexes an Obsidian vault into local ChromaDB and answers read-only questions about it: semantic search, tag queries, graph structure, and note retrieval. It never mutates the vault — for writes, use standard file tools or obsidian-cli and keep NoteBrain for the discovery step.

References, read on demand:
- [references/example.md](references/example.md) — 18 worked scenarios, exact commands, verified pitfalls.
- [references/flags.md](references/flags.md) — every flag, default, and config override.
- [references/schema.md](references/schema.md) — JSON/TSV output shape, `--jsonpath` use.

## Preflight (once per conversation)

```bash
notebrain stats --format=json
```

| Result | Do |
| ------ | -- |
| `chunks: 0` | The vault is unindexed — tell the user to run `notebrain ingest` first. Do not try to read the vault another way. |
| Binary missing / errors | Say NoteBrain isn't available and offer to check setup. Do not `grep`/`find` the vault instead. |
| `chunks > 0` | Proceed. |

**The config trap:** `~/.notebrain/config/config.toml` (or `--config`) overrides built-in flag defaults — `include-text`, `context-window`, `min-score`, `limit`, `top-k` all have config keys. If config enables text/context, every result carries `text`+`context` even without the flags. For lean output pass `--include-text=false --context-window=0`. If a query looks over-filtered, a configured `min-score` floor (e.g. `0.4`) means low-score rows never appear — that's expected, not a bug. Details: [flags.md](references/flags.md).

## Retrieval ladder

The vault is large; the context budget is not. Each step has one criterion that says **done**.

### Step 1 — Lean search

Determine the topic, then query it lean:

```bash
notebrain search "<topic>" --format json --include-text --context-window 1 --limit 3
```

**Done when** the top hit scores `>= 0.75` and its text fully answers the question. Then stop and answer — do not launch graph commands out of curiosity.

**Lean shapes:**
- Top candidates/slugs only: drop `--context-window`, use `--jsonpath="$.results[*].note_slug"`.
- Note-level (not chunk-level) list: `--group-by-note` to collapse to the best chunk per note; `dedupe` via `--jsonpath="$.results[*].note_slug" | sort -u`.
- Weak matches above the `--min-score 0.5` floor, or `--tag`, `--section`, `--has-tasks`, `--has-code`, `--exclude-note`.
- Multi-topic at once — boost by adding positional queries: `search "redis pubsub" "kafka brokers"`.
- A show-tags + `--jsonpath="$.results[0].tags"` reveals real note tags in one call.

Flag tables, `--min-score` semantics, filters: [flags.md](references/flags.md).

### Step 2 — Targeted depth

Only when the task needs **graph structure** or **related-but-unlinked** notes, pass the exact slug (see Slug discipline) to the single matching command. Pick one; don't run the whole ladder for a simple question.

| Intent | Command | Decisive part |
| ------ | ------- | ------------- |
| Reading / metadata only | `get` | `--meta` (header, no body) or `--head N` (first N chunks) — full `get` only on direct demand |
| What links to a note | `backlinks` | exactly the slug |
| What a note references / embeds | `refs` | kind filters; `--include-missing` for broken links |
| What's graph-neighbour | `connections` | `--hops 1–2` (exponential blow-up beyond) |
| Meaning-related but NOT linked | `hidden` | `--deep` for section-level matches |
| Related **including** linked | `hidden` | `--include-linked` |
| Concepts around a seed note | `boosted` | `--seed "<slug>"` (required) |
| Notes with tag X | `tags` | `--children` for the full family |
| What shares tags with note Y | `tags` | `--shared --min-shared N` |

**Done when** each command answers what you asked, or returns nothing — then go one rung **down** the ladder (reformulate, widen with `--limit`, or tag query), not up the filesystem.

### Lexical fallback explains the around-the-zero case

Semantic search returns zero results or nothing above `--min-score`, so `search` automatically falls back to a token scan over titles/paths/tags/text. Rows arrive `"lexical": true`, `score: 0`; the header prints `Lexical Search (no semantic matches)`. So a short word like `Lecture` can still hit. When even that returns nothing, lengthen the query into a descriptive phrase or switch to a `tags` query if the word is a heading/tag keyword. No fallback for `boosted` or `hidden`.

### Refs: what a note references

`refs` lists the note's attachments (images, PDFs, other) and external http(s) links, in first-occurrence order, read fresh from the note file on disk — no index staleness. It does NOT list links to other notes (that is `backlinks`/`connections`). Broken references are hidden by default; `--include-missing` surfaces them as `"missing": true`. External links are never missing and never touched over the network. No kind flags = every kind; the filters are pure kind selectors, no scores.

## Slug discipline

Slugs are the handle; titles are not. For graph, `get`, and `refs` commands, pass the exact `note_slug` returned by a prior `search`/`tags` — never a bare title, titles are ambiguous. Since the deterministic-resolution fix, a missing note is an **error** (`note not found: "<input>" ...`), not a silently guessed phantom slug. A "no indexed chunks" / "note not found" failure is normally a breadth-resolution problem, not a missing note. Slugs also go stale mid-conversation on schedule (cron re-ingest): if a slug that worked earlier now 404s, re-resolve via `search` before retrying.

## Tag discovery

Never guess a tag spelling — vault tags drift (`K8S` remembered vs `kubernetes` stored). Four rungs, stop where the answer arrives:

1. Enumerate cheaply: `tags --list --format tsv` (every tag + count). `--limit 0` = all; a config `limit` may cap — pass it explicitly.
2. From content: `search "<topic>" --limit 1 --show-tags --jsonpath="$.results[0].tags"`.
3. From the header: `get "<slug>" --format text`, read the `Tags:` line.
4. Then query: `tags "<tag>"` with `--children` for the whole family.

Tag semantics: `#` optional, case-insensitive, exact unless `--children` (then hierarchical prefix `kubernetes` ⊃ `kubernetes/cka`). JSON emits tags only with `--show-tags`, bare and lowercase — so in answer text render them as written.

## Response format

Lead with the answer; attach the sources; only embellish with threads the vault genuinely opens.

- **Direct question** — answer first, then list supporting notes as bullet titles under `**From the vault**`. Add 1–2 real follow-ups only if the vault covers them; skip padding when the answer is self-contained.
- **No result above `score 0.30`** — say so plainly. Offer 1–2 reformulations (synonyms, narrower/broader). Never pad weak matches; never go to the filesystem.
- **Weak/off-topic top hits** — demand precision: `--min-score 0.5` or add a distinguishing term. Short shorthands (`k8s`) are the usual cause; spell out the subject (`kubernetes`) before declaring the vault lacks it.
- **Traceability** — every fact claims a retrieved `note_slug`/`text`/`context`; never invent titles, paths, or quoted text. Label retrieved fact vs your own implication ("Your notes suggest…" vs "This looks like…"). Cite every note you lean on.