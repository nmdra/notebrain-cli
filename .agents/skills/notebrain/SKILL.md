---
name: notebrain-assistant
description: Use NoteBrain to search, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: MIT
compatibility: Requires the `notebrain` binary installed.
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain indexes an Obsidian vault into local ChromaDB for semantic search, graph traversal, and note retrieval.

## Scope & Boundaries

NoteBrain is **read-only** — it searches, retrieves, and explores notes that have already been indexed. It cannot create, rename, move, or edit notes. If the user's request requires writing or modifying vault files, use standard file tools (or the obsidian-cli skill if available) for those mutations, and use NoteBrain only for the discovery/search portion of the workflow.

## Pre-Flight: Verify NoteBrain Is Available

Before running your first query in a conversation, confirm NoteBrain is functional:

```bash
notebrain stats --format=json
```

- If the binary is missing or errors, tell the user plainly: _"NoteBrain doesn't appear to be installed or accessible. I can't search your vault without it."_ Do not fall back to `grep`/`find` against raw markdown files.
- If NoteBrain throws configuration or dependency errors, inform the user plainly so they can check their vault setup or configuration.
- If `stats` returns `0` chunks, the vault hasn't been indexed yet. Tell the user: _"Your vault hasn't been indexed. Run `notebrain ingest` first, then ask me again."_
- If `stats` succeeds with chunk counts > 0, proceed normally.

**Effective defaults are config-driven.** The CLI's built-in flag defaults (documented in [references/flags.md](references/flags.md)) can be overridden by `~/.notebrain/config/config.toml` (or `--config`) — notably `include-text`, `context-window`, `min-score`, `limit`, and `top-k`. If the config enables `include-text`/`context-window` (as the bundled example config does), every JSON result carries `text` and `context` **even without the flags**, which silently defeats token-saving assumptions. When you want lean output, pass the flags explicitly (`--include-text=false`, `--context-window=0`) instead of assuming built-in defaults. If a query seems over-filtered, check `min-score` in the config — a configured floor (e.g. `0.4`) means low-score matches never appear, which is expected behavior, not a search bug.

## Core Execution Principles

1. **NoteBrain Only — No Generic Filesystem Search**: Never use `grep`, `find`, `ls`, or ad-hoc shell scripts against markdown files. Treat `notebrain` as the sole interface to the vault. If a query returns nothing, refine the query (synonyms, broader/narrower phrasing) rather than falling back to bash.

2. **Session Caching & Reuse**: If `backlinks`, `connections`, or `hidden` was already executed for a given `note_slug` earlier in the conversation, reuse those results from context instead of re-querying — unless the user explicitly requests a fresh query or mentions they've just re-indexed/ingested the vault, in which case cached results may be stale.

   Cached slugs can also go stale mid-conversation: the vault may re-ingest on a schedule (cron/systemd) between your calls. If `get` 404s or `hidden --deep` reports "no indexed chunks" for a slug that worked earlier, re-resolve the slug with a fresh `search` before retrying.

3. **Prioritize `--context-window N` + `--include-text` Over Blind `get`**: Never blindly run `notebrain get <slug>` after a search hit. Full notes can be thousands of lines long; fetching entire notes floods context and wastes tokens. Instead, pass `--context-window N` (e.g., `--context-window 1` or `2`) on your `search`, `hidden`, or `boosted` queries to fetch ±N adjacent chunks around the match. Only use `get` when a task explicitly demands the entire note from start to finish.

   Need only metadata (tags, path, chunk count)? Prefer `get <slug> --meta` — it returns the header (title, path, tags, chunk count) with no note text. For note bodies, `get <slug> --head N` caps the text at the first N chunks while `Chunks` still reports the full total.

4. **Token-Efficient Extraction (`--jsonpath` & `tsv`)**:
   - Matching text snippets: `--jsonpath="$.results[*].text"`
   - Surrounding chunk context: `--jsonpath="$.results[*].context"`
   - When scanning tabular lists without text content, use `--format tsv` to drop repeating JSON key names.
   - When outputting full JSON (i.e., not using `--jsonpath`), `file_path` is included by default. Pass `--show-file-path=false` to hide it and cut token footprint by roughly 40–50%.
   - `--jsonpath` supports dotted paths, array wildcards (`[*]`), indices (`[0]`), slices (`[0:2]`), and recursive descent (`$..`) — but no jq-style pipe expressions, bracket-quoted keys, or filters. For multi-field extraction use `--format tsv` or two `--jsonpath` calls. Full dialect details in [references/flags.md](references/flags.md).
   - The `tags` field appears in JSON output only when `--show-tags` is passed (bare, lowercase — e.g. `["y4s2/ead"]`). Pass it when a query is tag-related.
   - One note can appear on multiple result rows (one per matched chunk). For distinct notes use `--group-by-note` (keeps the best chunk per note, adds `extra: "N matching chunks"`), `--top-k 1`, or dedupe slugs via `--jsonpath="$.results[*].note_slug" | sort -u` (piping `notebrain` stdout is fine — the no-shell rule covers vault files, not CLI output).

5. **Intelligent Query Splitting**: When researching compound questions or orthogonal topics (e.g., comparing two technologies), split the query into distinct positional arguments to activate multi-hit boosting:
   - **Positional arguments**: `notebrain search "redis pubsub" "kafka brokers" --limit 5 --format json`

6. **Avoid Blanket Chaining**: A single `search` with `--context-window 1 --include-text` answers most questions. Never blindly run `search → backlinks → connections → hidden` sequentially unless the user explicitly requests a comprehensive vault-wide audit of a topic. Pick the exact command tailored to the query.

7. **Keep Result Sets Small**: Default `--limit` and `--top-k` to 3–5. Larger result sets rarely add useful signal — they flood context with diminishing-relevance matches and inflate token costs. Only increase beyond 5 when the user explicitly asks for more results or the task requires exhaustive coverage (e.g., "list all notes tagged X").

8. **PDF Support**: By default, search results only return Markdown notes. If the user explicitly asks to include PDF notes in their search results, append the `--with-pdf` flag to `search` or `boosted` commands.

## Slug Discipline

Titles are ambiguous; slugs are not. For `get`, `backlinks`, `connections`, `hidden`, and `boosted`, always pass the exact `note_slug` returned by a prior `search`/`tags` call — never a bare title.

Title resolution is a fallback, and it can still mis-resolve on near-duplicate titles (e.g. "1. Java EE (EAD)" vs "1. Introduction to Java EE and EA"). Since the deterministic-resolution fix, a missing note is an error — `note not found: "<input>" — check the exact slug or title...` — instead of a silently guessed phantom slug. A command that fails with "no indexed chunks" or "note not found" is usually a resolution problem, not a missing note. If a command errors on a slug that worked earlier, the vault was likely re-ingested mid-conversation; re-resolve with a fresh `search`.

## Progressive Retrieval Workflow (`notebrain search`)

To prevent excessive tool calls, token bloat, and redundant queries, follow a two-step tiered retrieval:

### Step 1: Start Lean (Candidate & Slug Discovery)

```bash
notebrain search "<query>" --format=json --include-text
```

Check the `score` of your top candidates. If the top match has high similarity (`score ≥ 0.75`) and the text fully answers the user's question, **stop here**. Do not execute unnecessary follow-up queries.

If you've identified a candidate note but need surrounding paragraphs (±N chunks) to verify details:

```bash
notebrain search "<query>" --format=json --include-text --top-k 2 --context-window 1
```

| Flag                 | Purpose                                                                                                                       | Example |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------- | ------- |
| `--top-k N`          | Maximum chunks to retain **per note**. Prevents one long note from dominating results.                                        | `3`     |
| `--context-window N` | Includes ±N adjacent chunks around each match in the `context` field. Use for lightweight surrounding context across results. | `1`     |
| `--limit N`          | Maximum total number of results to return.                                                                                    | `5`     |

### Step 2: Escalate Conditionally (Deep Traversal & Connections)

Only when the task specifically requires exploring graph topology, backlinks, or implicit connections should you pass the discovered `note_slug` from Step 1 into specialized commands. Use the command reference below to pick the right one.

## Command Reference

| User Intent                                            | Command       | Syntax                                                                                 |
| ------------------------------------------------------ | ------------- | -------------------------------------------------------------------------------------- |
| "What do my notes say about X?"                        | `search`      | `notebrain search "topic" --context-window 1 --limit 3 --include-text`                 |
| "Skip specific note from search"                       | `search`      | `notebrain search "topic" --exclude-note "<note slug/Note title>"`                     |
| "Search notes filtered by a tag"                       | `search`      | `notebrain search "topic" --tag "y4s2/ead" --limit 5 --format json`                    |
| "Suppress weak matches"                                | `search`      | `notebrain search "topic" --min-score 0.3 --format json`                               |
| "Find the slug for a note about X" _(discovery step)_  | `search`      | `notebrain search "<query>" --jsonpath="$.results[*].note_slug"`                       |
| "Read full note Y" _(use sparingly; prefer context)_   | `get`         | `notebrain get "<slug-or-path>"`                                                       |
| "What links directly to this note?"                    | `backlinks`   | `notebrain backlinks "<slug>" --format json`                                           |
| "What is structurally nearby in the graph?"            | `connections` | `notebrain connections "<slug>" --hops 2 --format tsv`                                 |
| "What is related in meaning but NOT linked?"           | `hidden`      | `notebrain hidden "<slug>" --limit 5 --deep --format json`                             |
| "What is related in meaning (including linked notes)?" | `hidden`      | `notebrain hidden "<slug>" --include-linked --limit 5 --format json`                   |
| "Find concepts related to X centered around note Y"    | `boosted`     | `notebrain boosted --seed="<slug>" "query" --context-window 1 --limit 5 --format json` |
| "Find notes with tag X"                                | `tags`        | `notebrain tags "tag" --format json`                                                   |
| "Find notes with tag X and its child tags"             | `tags`        | `notebrain tags "tag" --children --format json`                                        |
| "What notes share tags with X?"                        | `tags`        | `notebrain tags "<slug>" --shared --min-shared 1 --format json`                        |

> **Need detailed flag descriptions or output schemas?** Read [references/flags.md](references/flags.md) for full flag tables and [references/schema.md](references/schema.md) for JSON envelope fields and TSV formatting. For result filters (`--section`, `--tag`, `--has-tasks`, `--has-code`, `--exclude-note`, `--min-score`, `--skip-phantom`), see the `search` table in references/flags.md.
>
> **Worked end-to-end scenarios?** Read [example.md](references/example.md) for major use cases with exact commands, expected outputs, and pitfalls.

## Tag Matching Rules

1. **Enumerate (cheapest when many tags)**: `notebrain tags --list --limit 0 --format tsv` — every indexed tag with its note count, one per line (`tag<TAB>count`). `--limit N` caps to the N most-used tags.
2. **Reveal tags from content (one call)**: `notebrain search "<topic>" --limit 1 --show-tags --jsonpath="$.results[0].tags"` — the topic finds a note that carries the real tag.
3. **Fallback — read the header**: `notebrain get "<slug>" --format text`, read the `Tags:` line.
4. **Then query**: `notebrain tags "<discovered tag>"` (optionally `--children` for the full family).

## Response Formahem separately whet

Match the response shape to the query type:

### Direct Questions

1. Answer the question first, in plain language.
2. List supporting notes underneath (note title only): `**From the vault**\n- Note Title`.
3. If the answer opens natural follow-up threads (related topics the vault covers, connections worth exploring), suggest 1–2. For simple factual lookups where the answer is self-contained, skip the follow-up — don't pad every response with questions that don't add value.

### No Relevant Results

If `search`, `hidden`, or `boosted` returns nothing above a usable score (`score < 0.30`):

- Say so plainly — don't pad the answer or overstate weak matches.
- Suggest 1–2 reformulated queries (synonyms, broader/narrower phrasing).
- Short common words (e.g. `Lecture`) now fall back to a lexical (token-based) scan when no semantic match clears the bar — rows arrive marked `"lexical": true` with `score: 0`. If the fallback also returns nothing, reformulate as a longer descriptive phrase, or switch to a `tags` query when the word is a heading/tag keyword.
- Do not fall back to filesystem search.

### Weak or Off-Topic Matches

If results come back but the top hits are semantically off-topic (e.g. cryptography notes for an "EAD" query at `score ≈ 0.45`):

- Re-run with `--min-score 0.5` (precision) — `notebrain search "<query>" --min-score 0.5 --format json`.
- Or reformulate the query to be more specific (add a distinguishing term) before concluding the vault lacks the topic.
- Short or multi-word shorthand queries (`Y2S4 EAD`) are the usual cause — spell out the subject (`enterprise application development`).

### General Rules

- Every factual claim must trace to a retrieved `note_slug` / `text` / `context` field — never invent titles, paths, or quoted text.
- Distinguish retrieved fact from your own inference explicitly (e.g., _"Your notes suggest..."_ vs. _"This looks like it connects to..."_).
- Cite every note referenced in the answer, even in a short direct-question response.
