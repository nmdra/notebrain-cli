# Commands Reference

NoteBrain provides a variety of commands to ingest, query, and analyze your Obsidian vault.

## `ingest`
Indexes your Obsidian vault into the local ChromaDB database.
```bash
notebrain ingest --vault "/path/to/vault" [--workers 4]
```
- Parses Markdown files.
- Extracts Wikilinks (`[[target]]`) and Tags (`#tag`).
- Chunks content and embeds via ONNX locally.
- *Note: Run this command whenever your vault has significantly changed.*

## `search`
Performs a semantic search against your vault chunks.
```bash
notebrain search "how do message brokers work?" --limit 5
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
