---
name: notebrain-assistant
description: Use NoteBrain to search, index, and explore an Obsidian vault via ChromaDB. Make sure to use this skill whenever the user mentions their notes, knowledge base, Obsidian vault, semantic search, finding connections, unlinked notes, or asks general exploratory questions like "what do I know about X", "find notes related to Y", "what connects to Z", or "summarize my notes on W", even if they don't explicitly mention NoteBrain, vector search, or ChromaDB.
license: MIT
allowed-tools: Bash(notebrain:*), Bash(./notebrain:*)
---

# NoteBrain CLI Skill for AI Agents

NoteBrain indexes an Obsidian vault into local ChromaDB for semantic search, graph traversal, and note retrieval.

## Quick Command Map

| User Intent                                         | Command       | Core Syntax Example                                                               |
| --------------------------------------------------- | ------------- | --------------------------------------------------------------------------------- |
| "What do my notes say about X?"                     | `search`      | `notebrain search "topic" --context-window 1 --limit 5 --include-text`            |
| "Read full note Y (Use sparingly; prefer context)"  | `get`         | `notebrain get "<slug-or-path>"`                                                  |
| "What links directly to this note?"                 | `backlinks`   | `notebrain backlinks "<slug>" --jsonpath="$.results[*].note_slug"`                |
| "What is structurally nearby in the graph?"         | `connections` | `notebrain connections "<slug>" --hops 2 --format tsv`                            |
| "What is related in meaning but NOT linked?"        | `hidden`      | `notebrain hidden "<slug>" --context-window 1 --limit 5 --include-text`           |
| "Find concepts related to X centered around note Y" | `boosted`     | `notebrain boosted --seed="<slug>" "query" --context-window 1 --limit 5`          |
| "What notes share tags with X?"                     | `tags`        | `notebrain tags "<slug>" --min-shared 1`                                          |
| "Index / check database status"                     | `ingest` / `stats`| `notebrain ingest` / `notebrain stats --format=json`                              |

> **Need specific flags or output schema?** Read [references/flags.md](file:///home/nimendra/Documents/Projects/obsidian-helper/.agents/skills/notebrain/references/flags.md) for full flag tables (filtering, top-k, context windows) and [references/schema.md](file:///home/nimendra/Documents/Projects/obsidian-helper/.agents/skills/notebrain/references/schema.md) for JSON envelope fields and TSV formatting.

---

## Core Execution Principles

1. **NoteBrain Only — No Generic Filesystem Search**: Never use `grep`, `find`, `ls`, or ad-hoc shell scripts against markdown files. Treat `notebrain` as the sole interface to the vault. If a query returns nothing, refine the query rather than falling back to bash.
2. **Prioritize `--context-window N` Over Blind `get`**: Never blindly run `notebrain get <slug>` after a search hit! In Obsidian vaults, full notes can be thousands of lines long; fetching entire notes floods your context window with irrelevant text and wastes massive tokens. Instead, pass `--context-window N` (e.g., `--context-window 1` or `2`) on your `search`, `hidden`, or `boosted` queries. This fetches ±N adjacent chunks around the match, giving you the exact surrounding context needed to answer the question without dumping the whole document into context. Only use `get` as a last resort when a task explicitly demands processing the entire note from start to finish.
3. **Token-Efficient Extraction (`--jsonpath` & `tsv`)**: Make `--jsonpath` your default tool for extracting targeted data! Instead of loading bulky JSON envelopes into context, append `--jsonpath` to extract exact scalar strings or arrays directly:
   - Extract matching text snippets: `--jsonpath="$.results[*].text"`
   - Extract surrounding chunk context: `--jsonpath="$.results[*].context"`
   - Extract note slugs for graph mapping: `--jsonpath="$.results[*].note_slug"`
   When scanning tabular lists without text, use `--format tsv` to drop repeating JSON key names.
4. **Non-Interactive Execution**: Always specify `--format json` (or `tsv`/`ndjson`/`--jsonpath`) on query commands to bypass the interactive TUI and receive structured data immediately.
5. **Intelligent Query Splitting (`--split`)**: When researching compound questions, long queries, or orthogonal topics (e.g., comparing two technologies), split the query into distinct terms using positional arguments (`notebrain search "redis pubsub" "kafka brokers"`) or `--split` (`notebrain search "redis, kafka" --split`). **Why?** NoteBrain's multi-hit boosting automatically ranks bridging notes above single-topic matches. For simple lookups, keep the query intact.
6. **CLI Syntax Rules**: In development environments, execute `./notebrain` if `notebrain` is not in PATH. Encapsulate queries and slugs in double quotes. Strictly use `--vault-path` and `--chroma-path` (never `--vault` or `--db`). For graph commands, pass exactly one positional argument: `<note>`.

---

## Progressive Retrieval Workflow

To prevent excessive tool calls and context bloat, follow a tiered retrieval strategy:

1. **Start Lean**: Always begin with a targeted search using `--context-window`:
   ```bash
   notebrain search "event driven architecture" --context-window 1 --top-k 2 --limit 5 --include-text --format json
   ```
2. **Check Score & Sufficiency**: If the top result's `score ≥ 0.75` and fully answers the question, **stop here**. Do not run further commands or blindly call `get`.
3. **Escalate Conditionally**: Only execute follow-up commands if the initial findings require it:
   - **Graph structure / nearby notes** → run `connections "<slug>" --hops 2 --jsonpath="$.results[*].note_slug"` (no `--include-text`).
   - **Incoming citations / what links here** → run `backlinks "<slug>" --format tsv`.
   - **Exploratory / conceptual bridges** → run `hidden "<slug>" --context-window 1 --limit 5 --include-text`.
4. **Avoid Blanket Chaining**: Never run all four commands (`search → backlinks → connections → hidden`) unless the user explicitly requests a comprehensive vault-wide audit of a topic.

---

## Session Caching & Reuse

If `backlinks`, `connections`, or `hidden` was already executed for a given note slug earlier in the conversation, reuse those results from context instead of re-querying, unless the user re-ingested the vault.
