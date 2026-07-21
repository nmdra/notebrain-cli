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

- If the binary is missing or errors, tell the user plainly: _"NoteBrain doesn't appear to be installed or accessible. I can't search your vault without it."_ Do not fall back to `grep`/`find` against raw markdown files — results would be incomplete and miss semantic matches.
- If `stats` returns `0` chunks, the vault hasn't been indexed yet. Tell the user: _"Your vault hasn't been indexed. Run `notebrain ingest --vault-path /path/to/vault` first, then ask me again."_
- If `stats` succeeds with chunk counts > 0, proceed normally.

## Core Execution Principles

1. **NoteBrain Only — No Generic Filesystem Search**: Never use `grep`, `find`, `ls`, or ad-hoc shell scripts against markdown files. Treat `notebrain` as the sole interface to the vault. If a query returns nothing, refine the query (synonyms, broader/narrower phrasing) rather than falling back to bash.

2. **Session Caching & Reuse**: If `backlinks`, `connections`, or `hidden` was already executed for a given `note_slug` earlier in the conversation, reuse those results from context instead of re-querying — unless the user explicitly requests a fresh query or mentions they've just re-indexed/ingested the vault, in which case cached results may be stale.

3. **Prioritize `--context-window N` + `--include-text` Over Blind `get`**: Never blindly run `notebrain get <slug>` after a search hit. Full notes can be thousands of lines long; fetching entire notes floods context and wastes tokens. Instead, pass `--context-window N` (e.g., `--context-window 1` or `2`) on your `search`, `hidden`, or `boosted` queries to fetch ±N adjacent chunks around the match. Only use `get` when a task explicitly demands the entire note from start to finish.

4. **Token-Efficient Extraction (`--jsonpath` & `tsv`)**:
   - Matching text snippets: `--jsonpath="$.results[*].text"`
   - Surrounding chunk context: `--jsonpath="$.results[*].context"`
   - When scanning tabular lists without text content, use `--format tsv` to drop repeating JSON key names.
   - When outputting full JSON (i.e., not using `--jsonpath`), the `file_path` field is omitted by default to cut token footprint by ~40–50%. Pass `--show-file-path` only if strictly needed.

5. **Intelligent Query Splitting**: When researching compound questions or orthogonal topics (e.g., comparing two technologies), split the query into distinct terms to activate multi-hit boosting:
   - **Positional arguments** (when exact terms are known): `notebrain search "redis pubsub" "kafka brokers" --limit 5 --format json`
   - **`--split` flag** (when splitting natural language by delimiters): `notebrain search "redis, kafka, rabbitmq" --split --limit 5 --format json`

6. **Avoid Blanket Chaining**: A single `search` with `--context-window 1 --include-text` answers most questions. Never blindly run `search → backlinks → connections → hidden` sequentially unless the user explicitly requests a comprehensive vault-wide audit of a topic. Pick the exact command tailored to the query.

7. **Keep Result Sets Small**: Default `--limit` and `--top-k` to 3–5. Larger result sets rarely add useful signal — they flood context with diminishing-relevance matches and inflate token costs. Only increase beyond 5 when the user explicitly asks for more results or the task requires exhaustive coverage (e.g., "list all notes tagged X").

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

| User Intent                                            | Command       | Syntax                                                                                           |
| ------------------------------------------------------ | ------------- | ------------------------------------------------------------------------------------------------ |
| "What do my notes say about X?"                        | `search`      | `notebrain search "topic" --context-window 1 --limit 3 --include-text`                 |
| "Find the slug for a note about X" _(discovery step)_  | `search`      | `notebrain search "<query>" --jsonpath="$.results[*].note_slug"`                                 |
| "Read full note Y" _(use sparingly; prefer context)_   | `get`         | `notebrain get "<slug-or-path>"`                                                                 |
| "What links directly to this note?"                    | `backlinks`   | `notebrain backlinks "<slug>" --format json`                                           |
| "What is structurally nearby in the graph?"            | `connections` | `notebrain connections "<slug>" --hops 2 --format tsv`                                           |
| "What is related in meaning but NOT linked?"           | `hidden`      | `notebrain hidden "<slug>" --limit 5 --deep --format json`                             |
| "What is related in meaning (including linked notes)?" | `hidden`      | `notebrain hidden "<slug>" --include-linked --limit 5 --format json`                   |
| "Find concepts related to X centered around note Y"    | `boosted`     | `notebrain boosted --seed="<slug>" "query" --context-window 1 --limit 5 --format json` |
| "Find notes with tag X"                                | `tags`        | `notebrain tags "#Tag" --format json`                                                  |
| "Find notes with tag X and its child tags"            | `tags`        | `notebrain tags "#Tag" --children --format json`                                      |
| "What notes share tags with X?"                        | `tags`        | `notebrain tags "<slug>" --shared --min-shared 1 --format json`                        |

> **Need detailed flag descriptions or output schemas?** Read [references/flags.md](references/flags.md) for full flag tables and [references/schema.md](references/schema.md) for JSON envelope fields and TSV formatting.

## Response Format

Match the response shape to the query type:

### Direct Questions

1. Answer the question first, in plain language.
2. List supporting notes underneath (note title only): `**From the vault**\n- Note Title`.
3. If the answer opens natural follow-up threads (related topics the vault covers, connections worth exploring), suggest 1–2. For simple factual lookups where the answer is self-contained, skip the follow-up — don't pad every response with questions that don't add value.

### No Relevant Results

If `search`, `hidden`, or `boosted` returns nothing above a usable score (`score < 0.30`):

- Say so plainly — don't pad the answer or overstate weak matches.
- Suggest 1–2 reformulated queries (synonyms, broader/narrower phrasing).
- Do not fall back to filesystem search.

### General Rules

- Every factual claim must trace to a retrieved `note_slug` / `text` / `context` field — never invent titles, paths, or quoted text.
- Distinguish retrieved fact from your own inference explicitly (e.g., _"Your notes suggest..."_ vs. _"This looks like it connects to..."_).
- Cite every note referenced in the answer, even in a short direct-question response.
