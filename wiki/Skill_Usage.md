# AI Agent Skill Usage

NoteBrain includes an optimized, built-in AI agent skill (`notebrain-assistant`) designed for AI coding assistants and autonomous agents (such as Google Antigravity / Gemini / Pi). This skill equips agents with seamless semantic search and knowledge graph traversal across your Obsidian vault.

## Where the Skill Lives

The skill instructions and evaluation workspace are located in the project root:

- **Skill Instructions**: `.agents/skills/notebrain/SKILL.md`

When working inside this repository or importing this skill into your global AI configuration, AI agents automatically discover and follow these instructions whenever you ask questions about your personal notes or knowledge base.

## How it Works: Tiered Retrieval Workflow

To prevent excessive token consumption and minimize latency, the `notebrain-assistant` skill strictly mandates a **Tiered Retrieval Workflow**:

1. **Lean Initial Search**:
   The agent always begins with a highly focused semantic query:
   ```bash
   notebrain search "<query>" --context-window 1 --top-k 2 --include-text
   ```
2. **Similarity Score Check**:
   If the top result returns a similarity score of `score >= 0.75` and provides sufficient context to answer your prompt, the agent **stops immediately** without making additional CLI calls.
3. **Conditional Escalation**:
   The agent only executes multi-step graph commands when explicitly needed:
   - **Graph Traversal**: For questions about relationships or connections, it runs `notebrain connections "<slug>" --hops 2` (metadata only, without `--include-text`).
   - **Backlinks**: For questions about what references a concept, it runs `notebrain backlinks "<slug>"`.
   - **Hidden Connections**: For discovering unlinked semantic bridges across your vault, it runs `notebrain hidden "<slug>"`.
4. **Session Caching**:
   Within a single conversation session, the agent caches and reuses previous query results rather than re-running identical CLI commands.

## Example Prompts

When pair programming with your AI assistant, try asking natural language questions like:

- _"What do my notes say about Kubernetes reconciliation loops? Summarize the main points."_
- _"Find notes connected to or linking to database-engineering within 2 hops."_
- _"Are there any hidden or unlinked concepts related to message broker backpressure in my vault?"_
- _"What connects to Redis Queue, and give me an overview of surrounding ideas."_
