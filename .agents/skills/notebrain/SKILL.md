---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: MIT
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain indexes an Obsidian vault into local ChromaDB for semantic search, graph traversal, and note retrieval.

## Session Caching & Reuse

If `backlinks`, `connections`, or `hidden` was already executed for a given note slug earlier in the conversation, reuse those results from context instead of re-querying, unless the user re-ingested the vault.

---

## Core Execution Principles

1. **NoteBrain Only — No Generic Filesystem Search**: Never use `grep`, `find`, `ls`, or ad-hoc shell scripts against markdown files. Treat `notebrain` as the sole interface to the vault. If a query returns nothing, refine the query rather than falling back to bash.

2. **Prioritize `--context-window N` + `--include-text` Over Blind `get`**: Never blindly run `notebrain get <slug>` after a search hit! In Obsidian vaults, full notes can be thousands of lines long; fetching entire notes floods your context window with irrelevant text and wastes massive tokens. Instead, pass `--context-window N` (e.g., `--context-window 1` or `2`) on your `search`, `hidden`, or `boosted` queries. This fetches ±N adjacent chunks around the match into `context` while specifically excluding the matched chunk itself. Only use `get` as a last resort when a task explicitly demands processing the entire note from start to finish.

3. **Non-Interactive & Quiet Execution (`--format json --compact`)**: Always specify `--format json --compact` (or `tsv`/`--jsonpath`) on non-interactive query commands. Adding `--compact` further reduces token footprint by stripping redundant envelope and result properties.

4. **Token-Efficient Extraction (`--jsonpath` & `tsv`)**: Make `--jsonpath` your default tool for extracting targeted data! Instead of loading bulky JSON envelopes into context, append `--jsonpath` to extract exact scalar strings or arrays directly:
   - Extract matching text snippets: `--jsonpath="$.results[*].text"`
   - Extract surrounding chunk context: `--jsonpath="$.results[*].context"`
   - Extract note slugs for graph mapping: `--jsonpath="$.results[*].note_slug"`
     When scanning tabular lists without text, use `--format tsv` to drop repeating JSON key names.

5. **Intelligent Query Splitting**: When researching compound questions or orthogonal topics (e.g., comparing two technologies), split the query into distinct terms. There are two ways to do this:
   - **Positional arguments** (preferred when you know the exact terms): `notebrain search "redis pubsub" "kafka brokers" --limit 5 --format json`
   - **`--split` flag** (preferred when splitting user-provided natural language by delimiters): `notebrain search "redis, kafka, rabbitmq" --split --limit 5 --format json`
     Both activate multi-hit boosting, which automatically ranks bridging notes (notes matching multiple sub-queries) above single-topic matches. For simple single-topic lookups, keep the query intact.

6. **Don't Chain Commands Unnecessarily**: A single `search` with `--context-window 1 --include-text` answers most questions. Only run follow-up commands (`backlinks`, `connections`, `hidden`) when the initial search is insufficient or the user explicitly asks for structural/graph exploration.
   - **Don't do this**: `search → backlinks → connections → hidden → tags` for a simple "what do my notes say about X?"
   - **Do this**: `search "X" --context-window 1 --limit 5 --include-text --format json` — check the results — stop if sufficient.

---

## JSON Output Schema & Token Efficiency

When `--format json` is used, the output envelope looks like this (scores are automatically rounded to 4 decimal places and query decorations are stripped):

```json
{
  "command": "search",
  "query": "event driven architecture",
  "total": 1,
  "results": [
    {
      "note_slug": "architectureevent-driven-systems",
      "title": "Event Driven Systems",
      "file_path": "Architecture/Event Driven Systems.md",
      "score": 0.852,
      "chunk_index": 2,
      "heading_path": "Overview > Message Brokers",
      "text": "Message brokers decouple producers from consumers...",
      "context": [
        "Producers publish events without knowing who consumes them...",
        "Consumers process events at their own pace..."
      ]
    }
  ]
}
```

Key fields: `note_slug` (use as input to graph commands), `score` (0–1 similarity, 4 decimal places), `text` (only with `--include-text`), `context` (only with `--context-window N`). For the full field specification, see [references/schema.md](references/schema.md).

---

## Progressive Retrieval Workflow (Example)

To prevent excessive tool calls and context bloat, follow a tiered retrieval strategy:

1. **Start Lean**: Always begin with a targeted search. (adjust filters based on user need)
   ```bash
   notebrain search "event driven architecture" --top-k 2 --limit 5 --format json --compact
   ```
2. **Check Score & Sufficiency**: If the top result's `score ≥ 0.75` and fully answers the question, **stop here**. Do not run further commands.
3. **Escalate Conditionally**: Only execute follow-up commands if the initial findings require it:
   - **Graph structure / nearby notes** → run `connections "<slug>" --hops 2 --jsonpath="$.results[*].note_slug"` (no `--include-text`).
   - **Incoming citations / what links here** → run `backlinks "<slug>" --format tsv`.
   - **Exploratory / conceptual bridges** → run `hidden "<slug>" --context-window 1 --limit 5 --include-text`.
4. **Avoid Blanket Chaining**: Never run all four commands (`search → backlinks → connections → hidden`) unless the user explicitly requests a comprehensive vault-wide audit of a topic.

---

## Quick Command Map

| User Intent                                            | Command       | Core Syntax Example                                                       |
| ------------------------------------------------------ | ------------- | ------------------------------------------------------------------------- |
| "What do my notes say about X?"                        | `search`      | `notebrain search "topic" --context-window 1 --limit 3 --include-text`    |
| "Read full note Y (Use sparingly; prefer context)"     | `get`         | `notebrain get "<slug-or-path>"`                                          |
| "What links directly to this note?"                    | `backlinks`   | `notebrain backlinks "<slug>" --jsonpath="$.results[*].note_slug"`        |
| "What is structurally nearby in the graph?"            | `connections` | `notebrain connections "<slug>" --hops 2 --format tsv`                    |
| "What is related in meaning but NOT linked?"           | `hidden`      | `notebrain hidden "<slug>" --context-window 1 --limit 5 --include-text`   |
| "What is related in meaning (including linked notes)?" | `hidden`      | `notebrain hidden "<slug>" --include-linked --context-window 1 --limit 5` |
| "Find concepts related to X centered around note Y"    | `boosted`     | `notebrain boosted --seed="<slug>" "query" --context-window 1 --limit 5`  |
| "What notes share tags with X?"                        | `tags`        | `notebrain tags "<slug>" --min-shared 1`                                  |

> **Need specific flags or output schema?** Read [references/flags.md](references/flags.md) for full flag tables (filtering, top-k, context windows) and [references/schema.md](references/schema.md) for JSON envelope fields and TSV formatting.
