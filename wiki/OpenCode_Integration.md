# OpenCode Agent Integration Guide

[OpenCode](https://opencode.ai) is an open-source AI coding and terminal agent built for autonomous development and multi-agent workflows. By configuring NoteBrain as a dedicated OpenCode agent (`notebrain-chat`), you can transform your Obsidian vault into an interactive, grounded semantic knowledge base for your AI pair programming sessions.

---

## Why Use NoteBrain with OpenCode?

Obsidian vaults often contain thousands of interlinked notes, design docs, code snippets, and meeting records. Traditional AI agents struggle with vaults because:
1. **Context Window Flooding**: Blindly running `grep` or reading full markdown files (`cat` / `view_file`) dumps thousands of irrelevant lines into context, skyrocketing token consumption.
2. **Missing Semantic Context**: Keyword search (`grep`) misses synonyms, conceptual bridges, and structural graph hops (`wikilinks`).

NoteBrain solves this by querying a local **ChromaDB HNSW vector index**. When combined with OpenCode's granular permission system (`permission` block), you can sandbox the agent so it strictly interacts with your vault through high-precision semantic queries (`notebrain search`, `hidden`, `connections`, `backlinks`).

---

## Agent Configuration Overview (`notebrain-chat`)

In OpenCode, agent configurations are Markdown files stored inside `.opencode/agents/` (for project-specific agents) or `~/.config/opencode/agents/` (for global agents across all repositories).

An OpenCode agent file consists of two sections:
1. **YAML Frontmatter**: Defines agent metadata (`name`, `description`, `mode`, `temperature`, `model`) and the strict **`permission` sandbox**.
2. **Markdown System Instructions**: Defines the agent's role, operating rules, tiered retrieval strategy, and required response formatting.

### The Power of the `permission` Sandbox

The `permission` block is the most critical feature of the OpenCode configuration:
```yaml
permission:
  bash:
    "*": deny
    "notebrain *": allow
    "./notebrain *": allow

  edit: deny
  glob: deny
  grep: deny
  webfetch: deny
  websearch: deny
  task: deny
  todowrite: deny
  lsp: deny
  skill: deny
```

By explicitly denying `edit`, `glob`, `grep`, `webfetch`, and all wildcard shell commands (`bash: "*": deny`) while allowing only `"notebrain *": allow` and `"./notebrain *": allow`, OpenCode structurally enforces NoteBrain's **Core Principle #1 (No Generic Filesystem Search)**. The agent is physically restricted from running ad-hoc `grep`, `find`, `ls`, or `cat` loops against your Markdown files on disk, ensuring 100% of retrieval runs through NoteBrain's token-optimized vector engine.

---

## Complete Configuration (`notebrain-chat.md`)

To set up your NoteBrain assistant in OpenCode, create a file named `.opencode/agents/notebrain-chat.md` (or `~/.config/opencode/agents/notebrain-chat.md`) and paste the following complete configuration:

```markdown
---
name: notebrain-chat
description: Use NoteBrain to search, summarize, and explore an Obsidian vault. Invoke this agent whenever the user asks about their notes, knowledge base, Obsidian vault, semantic search, related ideas, graph relationships, or wants to discover, summarize, or connect information from their vault.
mode: primary
temperature: 0.3
model: openrouter/tencent/hy3:free

permission:
  bash:
    "*": deny
    "notebrain *": allow
    "./notebrain *": allow

  edit: deny
  glob: deny
  grep: deny
  webfetch: deny
  websearch: deny
  task: deny
  todowrite: deny
  lsp: deny
  skill: deny
---

# Role

You are a semantic knowledge retrieval assistant for Obsidian vaults.

Answer questions using NoteBrain results rather than general knowledge. Every factual statement about the vault should be supported by retrieved notes.

---

# Operating Rules

## Use NoteBrain exclusively

Never inspect markdown files directly or use filesystem search tools.

If retrieval is weak, improve the semantic query instead of bypassing NoteBrain.

---

## Retrieve the minimum information needed

Always start with `search`.

Prefer:

- `--context-window`
- `--include-text`
- `--format json --compact`

Use `--jsonpath` or TSV whenever only specific fields are needed.

Only use `get` if the user explicitly requests the entire note or the complete note is required.

---

## Retrieval Strategy

Start with:

```
search
```

If the results answer the user's question:

→ Respond.

Otherwise perform only the additional retrieval required.

| User needs | Command |
|------------|---------|
| Full note | `get` |
| Incoming links | `backlinks` |
| Graph neighbors | `connections` |
| Semantic neighbors | `hidden` |
| Search around a note | `boosted` |
| Shared tags | `tags` |

Do not chain graph commands automatically.

---

## Search Guidelines

Prefer semantic intent over literal keywords.

If results are weak:

- simplify the query
- use synonyms
- split unrelated topics into separate searches

Reuse previous `connections`, `backlinks`, and `hidden` results during the conversation unless the vault has been re-indexed.

---

# Response Rules

- Answer the user's question first.
- Base factual claims only on retrieved notes.
- Clearly distinguish retrieved facts from your own interpretation.
- Never invent note titles, paths, or quotations.
- Cite every supporting note.
- If nothing relevant is found, say so and suggest one or two alternative searches.

---

# Response Formats

### Direct Question

Answer first.

Then include:

**From your vault**

- Note Title — File Path

---

### Exploration

Include:

- Major themes
- Relationships between notes
- Supporting notes
- Suggested follow-up searches

---

### Inventory

- Group notes by topic.
- Include note counts.
```

---

## Invoking the Agent in OpenCode

Once saved, you can interact with your `notebrain-chat` agent using two primary workflows:

1. **Explicit Mode Selection (`primary` mode)**:
   Because `mode: primary` is set, you can select `notebrain-chat` as your active primary agent in the OpenCode CLI session (or invoke `/notebrain-chat` directly). All prompts typed into the chat will be answered exclusively using your Obsidian vault.
2. **Automatic Router Delegation (`subagent` mode)**:
   If you change `mode: primary` to `mode: subagent` (or let a master primary agent manage subagents), OpenCode's routing engine reads the `description` frontmatter:
   > _"Use NoteBrain to search, summarize, and explore an Obsidian vault. Invoke this agent whenever the user asks about their notes, knowledge base..."_
   
   Whenever you ask your coding agent questions like _"What architectural decisions did I write down about Redis Streams in my vault?"_, OpenCode will automatically spawn `notebrain-chat` in the background, run the semantic query via `notebrain search`, and return grounded citations back to your coding context!

---

## Token Efficiency Best Practices

When `notebrain-chat` executes commands via the allowed `notebrain *` bash permission, NoteBrain automatically applies several optimizations for LLM consumption:
- **Automatic Quiet Mode (`--quiet`)**: When `--format=json`, `tsv`, `ndjson`, or `--jsonpath` is used, NoteBrain automatically suppresses embedder loading spinners and progress logs (`WithQuiet`). This guarantees that OpenCode receives clean, parseable JSON without TUI corruption.
- **Compact JSON (`--compact`)**: Strips redundant properties (`command`, `file_path`) while keeping essential metadata (`note_slug`, `title`, `score`, `text`, `context`).
- **Non-Redundant Context Windows**: Passing `--context-window 1` fetches surrounding chunks around the match while excluding the matched chunk (`text`) itself from the `context` array, preventing duplicate tokens.
