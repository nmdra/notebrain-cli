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

## Invoking the Agent in OpenCode

Once saved, you can interact with your `notebrain-chat` agent using two primary workflows:

1. **Explicit Mode Selection (`primary` mode)**:
   Because `mode: primary` is set, you can select `notebrain-chat` as your active primary agent in the OpenCode CLI session (or invoke `/notebrain-chat` directly). All prompts typed into the chat will be answered exclusively using your Obsidian vault.
2. **Automatic Router Delegation (`subagent` mode)**:
   If you change `mode: primary` to `mode: subagent` (or let a master primary agent manage subagents), OpenCode's routing engine reads the `description` frontmatter:

   > _"Use NoteBrain to search, summarize, and explore an Obsidian vault. Invoke this agent whenever the user asks about their notes, knowledge base..."_

   Whenever you ask your coding agent questions like _"What architectural decisions did I write down about Redis Streams in my vault?"_, OpenCode will automatically spawn `notebrain-chat` in the background, run the semantic query via `notebrain search`, and return grounded citations back to your coding context!

## Example

[Notebrain Chat](./Notebrain-Chat.md)
