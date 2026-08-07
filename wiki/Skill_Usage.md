# AI Agent Skill Usage

NoteBrain has a built-in AI agent skill: `notebrain-assistant`. This skill operates with AI coding assistants and autonomous agents (for example, Google Antigravity, Opencode, Claude Code, and Pi). The skill gives agents semantic search and knowledge graph traversal for your Obsidian vault.

## Location of the Skill

The skill instructions and the evaluation workspace are in the project root:

- **Skill Instructions**: `.agents/skills/notebrain/SKILL.md`
- **Workspace snapshot**: `.agents/skills/notebrain-workspace/iteration-3/skill-snapshot/SKILL.md` (an identical copy)

AI agents find and obey these instructions automatically when you ask about your notes. This occurs when you work inside this repository or import the skill into your AI configuration.

## Tiered Retrieval Workflow

The `notebrain-assistant` skill mandates a tiered retrieval workflow. This workflow prevents token waste and decreases latency.

1. **Lean Initial Search and Quiet Mode**:
   The agent starts with a focused semantic query:

   ```bash
   notebrain search "<query>" --context-window 1 --top-k 2 --include-text --format json
   ```

   NoteBrain writes results to stdout and all diagnostics (progress, warnings) to stderr, so `--format json` output is always clean machine JSON. The `--context-window N` flag gets adjacent chunks but removes the matched chunk from the context. This decreases token consumption.

2. **Similarity Score Check**:
   If the top result has a similarity score of `score >= 0.75` (for example, `0.8520`) and gives enough context to answer you, the agent stops.

3. **Conditional Escalation**:
   The agent executes multi-step graph commands only when necessary:
   - **Graph Traversal**: To find connections, the agent runs `notebrain connections "<slug>" --hops 2 --format tsv`.
   - **Backlinks**: To find references to a concept, the agent runs `notebrain backlinks "<slug>" --format tsv`.
   - **Hidden Connections**: To find unlinked semantic bridges, the agent runs `notebrain hidden "<slug>" --context-window 1 --include-text --format json`.

4. **Session Caching**:
   The agent caches previous query results (connections, backlinks, and hidden) in one conversation session. The agent does not execute identical CLI commands again.

> [!TIP]
> Do you use NoteBrain with OpenCode? Read the [OpenCode Agent Integration Guide](OpenCode_Integration.md). This guide explains how to configure `notebrain-chat` with strict sandbox permissions and custom agent rules.

## Example Prompts

When you pair program with an AI assistant, you can ask questions like:

- _"What do my notes say about Kubernetes reconciliation loops? Summarize the main points."_
- _"Find notes connected to or linking to database-engineering within 2 hops."_
- _"Are there any hidden or unlinked concepts related to message broker backpressure in my vault?"_
- _"What connects to Redis Queue, and give me an overview of surrounding ideas."_

The skill contains a worked scenario guide with exact commands, expected output shapes, and pitfalls: `.agents/skills/notebrain/references/example.md`. It covers tag discovery, semantic search, graph traversal, extraction, and failure recovery.
