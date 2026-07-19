---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: MIT
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain indexes an Obsidian vault into local ChromaDB for semantic search, graph traversal, and note retrieval.

## Core Execution Principles

1. **NoteBrain Only — No Generic Filesystem Search**: Never use `grep`, `find`, `ls`, or ad-hoc shell scripts against markdown files. Treat `notebrain` as the sole interface to the vault. If a query returns nothing, refine the query rather than falling back to bash.

2. **Session Caching & Reuse**: If `backlinks`, `connections`, or `hidden` was already executed for a given `note_slug` earlier in the conversation, reuse those results from context instead of re-querying, unless the user explicitly requests a fresh query.

3. **Prioritize `--context-window N` + `--include-text` Over Blind `get`**: Never blindly run `notebrain get <slug>` after a search hit! Full notes can be thousands of lines long; fetching entire notes floods your context window and wastes tokens. Instead, pass `--context-window N` (e.g., `--context-window 1` or `2`) on your `search`, `hidden`, or `boosted` queries to fetch `±N` adjacent chunks around the match while excluding the matched chunk itself. Only use `get` when a task explicitly demands processing the entire note from start to finish.

4. **Token-Efficient Extraction (`--jsonpath` & `tsv`)**: Make `--jsonpath` your default tool for extracting targeted data without loading bulky JSON envelopes into context:
   - Matching text snippets: `--jsonpath="$.results[*].text"`
   - Surrounding chunk context: `--jsonpath="$.results[*].context"`
     When scanning tabular lists without text, use `--format tsv` to drop repeating JSON key names. Always add `--compact` when outputting full JSON.

5. **Intelligent Query Splitting**: When researching compound questions or orthogonal topics (e.g., comparing two technologies), split the query into distinct terms to activate multi-hit boosting:
   - **Positional arguments** (when exact terms are known): `notebrain search "redis pubsub" "kafka brokers" --limit 5 --format json`
   - **`--split` flag** (when splitting natural language by delimiters): `notebrain search "redis, kafka, rabbitmq" --split --limit 5 --format json`

6. **Avoid Blanket Chaining**: A single `search` with `--context-window 1 --include-text` answers most questions. Never blindly run `search → backlinks → connections → hidden` sequentially unless the user explicitly requests a comprehensive vault-wide audit of a topic. Pick the exact tool tailored to the query.

7. **Strict Rule**: Keep `--limit` and `--top-k` under 5 unless the user requests otherwise.

## Example Progressive Retrieval Workflow (`notebrain search`)

To prevent excessive tool calls, token bloat, and redundant queries, follow a tiered retrieval strategy:

1. **Step 1: Start Lean (Candidate & Slug Discovery)**

   ```bash
   notebrain search "<query>" --format=json --compact --include-text=true
   ```

   Check the `score` of your top candidates. If the top match has high similarity (`score ≥ 0.75`) and the metadata fully answers the user's question, **stop here**. Do not execute unnecessary follow-up queries.

   If you have identified a candidate note but require the full markdown chunk or surrounding paragraphs (`±N` chunks) to verify the details before answering or escalating.

   ```bash
   notebrain search "<query>" --format=json --compact --include-text=true --top-k 2 --context-window 1
   ```

| Flag                 | Purpose                                                                                                                                                                                         | example value |
| -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------- |
| `--top-k N`          | Maximum number of chunks to retain **per note**. Prevents a single long note from dominating results while preserving coverage across multiple notes.                                           | `3`           |
| `--context-window N` | Includes ±N adjacent chunks around each matched chunk in the `context` field. Useful for lightweight surrounding context across multiple results; use `get` when full note content is required. | `1`           |
| `--limit N`          | Maximum total number of results to return.                                                                                                                                                      | `7`           |

2. **Step 4: Escalate Conditionally (Deep Traversal & Connections)**
   Only when the task specifically requires exploring graph topology, backlinks, or implicit connections should you pass the discovered `note_slug` from Step 1 into specialized traversal commands (`backlinks`, `connections`, `hidden`, or `boosted`). See the exact syntax options in the **Example Commands** section below.

## Response Format

Match the response shape to the query type:

### Direct Questions

1. Answer the question first, in plain language.
2. List supporting notes underneath (only use note title). (`**From the vault**\n- Note Title`).
3. Follow up questions.

### No Relevant Results

If `search`, `hidden`, or `boosted` returns nothing above a usable score (`score < 0.30`):

- Say so plainly — don't pad the answer or overstate weak matches.
- Suggest 1–2 reformulated queries (synonyms, broader/narrower phrasing) instead of falling back to filesystem search.

### General Rules

- Every factual claim must trace to a retrieved `note_slug` / `text` / `context` field — never invent titles, paths, or quoted text.
- Distinguish retrieved fact from your own inference explicitly (e.g., _"Your notes suggest..."_ vs. _"This looks like it connects to..."_).
- Cite every note referenced in the answer, even in a short direct-question response.

## Quick Command Map

| User Intent                                            | Command       | Core Syntax Example                                                                               |
| ------------------------------------------------------ | ------------- | ------------------------------------------------------------------------------------------------- |
| "What do my notes say about X?"                        | `search`      | `notebrain search "topic" --context-window 1 --limit 3 --include-text`                            |
| "Read full note Y (Use sparingly; prefer context)"     | `get`         | `notebrain get "<slug-or-path>"`                                                                  |
| "What links directly to this note?"                    | `backlinks`   | `notebrain backlinks "<slug>" --jsonpath="$.results[*].note_slug"`                                |
| "What is structurally nearby in the graph?"            | `connections` | `notebrain connections "<slug>" --hops 2 --format tsv`                                            |
| "What is related in meaning but NOT linked?"           | `hidden`      | `notebrain hidden "<slug>" --limit 5 --deep` (use `--deep` flag for chunk-by-chunk deep analysis) |
| "What is related in meaning (including linked notes)?" | `hidden`      | `notebrain hidden "<slug>" --include-linked --limit 5`                                            |
| "Find concepts related to X centered around note Y"    | `boosted`     | `notebrain boosted --seed="<slug>" "query" --context-window 1 --limit 5`                          |
| "What notes share tags with X?"                        | `tags`        | `notebrain tags "<slug>" --min-shared 1`                                                          |

> **Need specific flags or output schema?** Read [references/flags.md](references/flags.md) for full flag tables (filtering, top-k, context windows) and [references/schema.md](references/schema.md) for JSON envelope fields and TSV formatting.

## Example Commands

### Find note Slug only

Use the following command to find the note slug by searching for a query. Run this command before using any command (hidden, connections, tags, or backlinks) that requires a note slug as input.

```bash
notebrain search "<query>" \
  --jsonpath="$.results[*].note_slug"
```

### Incoming links

```bash
notebrain backlinks "<slug>" \
  --jsonpath="$.results[*].note_slug"
```

### Related semantic neighbors (Hidden Connections)

```bash
notebrain hidden "<slug>" \
  --limit 5 \
  --deep
```

### Graph neighbors

```bash
notebrain connections "<slug>" \
  --hops 2 \
  --format tsv
```

### Boost search around a note

```bash
notebrain boosted \
  --seed="<slug>" \
  "<query>" \
  --limit 5
```
