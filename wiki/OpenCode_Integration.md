# OpenCode Agent Integration Guide

[OpenCode](https://opencode.ai) is an open-source AI coding and terminal agent. It supports autonomous development and multi-agent workflows. When you configure NoteBrain as a dedicated OpenCode agent (`notebrain-chat`), you make your Obsidian vault an interactive semantic knowledge base for your AI pair programming sessions.

---

## Why Use NoteBrain with OpenCode?

Obsidian vaults have thousands of interlinked notes, design documents, code snippets, and meeting records. Traditional AI agents cannot process these vaults correctly because:

1. **Context window flooding**: The agents run `grep` or read full markdown files. This puts thousands of irrelevant lines into the context and increases token consumption.
2. **Missing semantic context**: Keyword search (`grep`) does not find synonyms, conceptual bridges, or structural graph hops (`wikilinks`).

NoteBrain avoids these problems because it queries a local ChromaDB HNSW vector index. When you use the permission system of OpenCode, you can sandbox the agent. Then the agent uses only high-precision semantic queries (`notebrain search`, `hidden`, `connections`, `backlinks`).

---

## Agent Configuration (`notebrain-chat`)

In OpenCode, agent configurations are Markdown files. They are in `.opencode/agents/` (for project-specific agents) or `~/.config/opencode/agents/` (for global agents).

An OpenCode agent file has two parts:

1. **YAML Frontmatter**: This part gives agent metadata (`name`, `description`, `mode`, `temperature`, `model`) and the strict `permission` sandbox.
2. **Markdown System Instructions**: This part gives the role of the agent, its operating rules, its tiered retrieval strategy, and the required response formatting.

## Invoke the Agent in OpenCode

When the configuration is complete, you can interact with your `notebrain-chat` agent in two ways:

1. **Explicit Mode Selection (`primary` mode)**:
   Because `mode: primary` is set, you can select `notebrain-chat` as your active primary agent in the OpenCode CLI session. Alternatively, you can use the `/notebrain-chat` command. The agent answers all prompts with data from your Obsidian vault.

2. **Automatic Router Delegation (`subagent` mode)**:
   If you change `mode: primary` to `mode: subagent`, the routing engine reads the `description` frontmatter:

   > _"Use NoteBrain to search, summarize, and explore an Obsidian vault. Invoke this agent whenever the user asks about their notes, knowledge base..."_

   When you ask a question like _"What architectural decisions did I write down about Redis Streams in my vault?"_, OpenCode spawns `notebrain-chat` in the background automatically. The agent executes the semantic query with `notebrain search` and sends citations back to your coding context.

## Example

[Notebrain Chat](./Notebrain-Chat.md)
