# Commands Reference

NoteBrain provides a variety of commands to ingest, query, and analyze your Obsidian vault.

```text
Usage: notebrain <command> [flags]

Index and search your Obsidian vault with semantic intelligence

Flags:
  -h, --help             Show context-sensitive help.
      --chroma-path="~/.notebrain/chroma"
                         path to ChromaDB persistent storage
      --chroma-mode="persistent"
                         ChromaDB client mode ('persistent' or 'http')
      --chroma-url="http://localhost:8000"
                         ChromaDB server URL (used when --chroma-mode=http)
      --vault-path=STRING    Obsidian vault path (also used as vault name fallback)
      --verbose              enable verbose output
      --no-hyperlinks        Disable OSC 8 terminal hyperlinks in output
      --format="text"        output format (text, json, tsv, ndjson)
      --jsonpath=STRING      JSONPath expression for filtering output (e.g., $.results[0].note_slug)
      --include-text         include matched chunk text in structured output
      --min-score=0          suppress results below this similarity score (0–1)
      --respect-exclude      respect Obsidian userIgnoreFilters and attachmentFolderPath settings during ingest (default: true)
      --use-editor           enable external editor ($EDITOR) integration as default open type (default: false)
      --config="~/.notebrain/config/config.toml"
                             Path to config file

Commands:
  ingest         Ingest markdown files from a vault
  search         Semantic search across indexed notes
  get            Retrieve the full reconstructed markdown text of a note
  backlinks      Find incoming links to a note
  connections    Traverse graph connections
  hidden         Discover hidden semantic links between unlinked notes
  tags           Find notes sharing common tags
  boosted        Graph-boosted semantic search
  stats          Show collection statistics
  reset          Reset the ChromaDB collections
```

### 🪄 Interactive Terminal UI
For all query commands (`search`, `backlinks`, `connections`, `hidden`, `boosted`, `tags`), NoteBrain will launch a **beautiful interactive terminal UI**! 
- Use the **Up/Down** arrow keys to navigate the results.
- Press **/** to live-filter the results.
- Press **Enter** to open the selected note using your default method (Obsidian, or your editor if `--use-editor` is enabled).
- Press **o** to open the note explicitly in Obsidian.
- Press **e** to open the note explicitly in your terminal or GUI editor (defined by the `$EDITOR` environment variable).
- Press **q** or **Esc** to exit.

## `ingest`
Indexes your Obsidian vault into the local ChromaDB database.
```bash
notebrain ingest --vault-path "/path/to/vault" [--workers 4]
```
- Parses Markdown files.
- Extracts Wikilinks (`[[target]]`) and Tags (`#tag`).
- Chunks content and embeds via ONNX locally.
- *Note: Run this command whenever your vault has significantly changed.*
- *Tip: You can automate periodic background indexing using OS cron jobs or systemd timers (see [Scheduled Ingestion](Scheduled_Ingestion.md)).*

## `search`
Performs a semantic search against your vault chunks. Supports optional tag filtering via `--tag`.
```bash
notebrain search "how do message brokers work?" --limit 5
notebrain search "kubernetes reconciliation" --tag="Kubernetes" --limit 5
```

## `get`
Retrieves and reconstructs the full markdown content of a note by combining all indexed chunks matching the given note slug or file path.
```bash
notebrain get "02areaskubernetesckadkubernetes-native-applications"
```

## `backlinks`
Finds notes linking to a given note. Replaces the native Obsidian backlinks panel but queries directly via the local graph index.
```bash
notebrain backlinks "Redis"
```

## `connections`
Finds notes connected via a breadth-first graph traversal.
```bash
notebrain connections "Redis" --hops 2
```
- `--hops`: Defines the depth of the graph traversal (default is 2).

## `hidden`
Discovers "hidden connections" — notes that are semantically close to the given note but have no direct wikilink relationship in Obsidian.
```bash
notebrain hidden "Redis" --limit 5
```

## `boosted`
Graph-boosted semantic search. Performs a semantic query, but boosts the score of chunks that are structurally connected to a "seed" note.
```bash
notebrain boosted "message queues and broker" --seed "Redis" --boost 2.0 --limit 5
```
- `--seed`: The origin note for graph boosting.
- `--boost`: The multiplier applied to graph-connected results.

## `tags`
Find notes sharing tags with a given note.
```bash
notebrain tags "Redis" --min-shared 1
```

## `stats`
Displays statistics for the ChromaDB collections used by NoteBrain.
```bash
notebrain stats
```

## `reset`
Drops all collections and starts fresh. This operation is irreversible.
```bash
notebrain reset
```

## Configuration File

You can set any global flag persistently by creating a `config.toml` file at `~/.notebrain/config/config.toml` (or passing `--config=/path/to/config.toml`). 

Flags are mapped implicitly to TOML keys (without the `--` prefix). NoteBrain supports normalized configuration keys, meaning `snake_case` (`vault_path`) and `kebab-case` (`vault-path`) work interchangeably.

```toml
vault-path = "/path/to/my/Second Brain"
vault-name = "Second Brain"
format = "json"
chroma-path = "~/.notebrain/chroma"
verbose = true
context-window = 1
```

## Machine-Readable Output & AI Agent Chaining

NoteBrain supports structured `snake_case` JSON, TSV, and NDJSON outputs (`note_slug`, `title`, `file_path`, `score`, `tags`, `text`) for clean automation and AI agent workflows. The TUI is automatically suppressed when formatting is not `text`.

```bash
notebrain search "golang concurrency" --format json --include-text
```

For direct shell pipeline extraction without external JSON tools like `jq`, use `--jsonpath`:

```bash
# 1. Extract note slug directly
SLUG=$(notebrain search "message broker backpressure" --limit 1 --jsonpath="$.results[0].note_slug")

# 2. Fetch full note text or related backlinks
notebrain get "$SLUG" --jsonpath="$.text"
notebrain backlinks "$SLUG" --jsonpath="$.results[*].note_slug"
```
