# NoteBrain MCP server

Start one local server per MCP client:

```sh
notebrain mcp serve
```

The client starts the process and communicates through stdin and stdout. There is no HTTP listener. For clients that accept an `mcpServers` configuration, use:

```json
{
  "mcpServers": {
    "mycli": {
      "command": "notebrain",
      "args": ["mcp", "serve"]
    }
  }
}
```

If the client cannot find NoteBrain on its PATH, use an absolute binary path for `command`. To use a custom TOML file or change the concurrency cap, use startup arguments such as:

```json
{
  "mcpServers": {
    "mycli": {
      "command": "/usr/local/bin/notebrain",
      "args": ["mcp", "serve", "--config=/home/me/.notebrain/config/config.toml", "--max-concurrent-requests=2"]
    }
  }
}
```

## Prerequisites and configuration

- Install NoteBrain and its local Chroma runtime. Index the vault with the ordinary CLI before requesting note data. See [Installation](Installation.md).
- Model-dependent searches also require the installed ONNX runtime and embedding model. Discovery does not need a vault, index, native runtime, or model. MCP does not install dependencies or create a missing store.
- Startup flags override TOML. You can set `--chroma-path`, `--vault-path`, and `--vault-name` at startup. Restart the server to apply configuration edits. Tool arguments cannot change paths or output format.
- Results include the equivalent of the CLI JSON envelope as structured content and as a compact JSON text block. The server reserves stdout for the protocol. Startup diagnostics go to stderr. MCP does not use the configured CLI log file.

## Retrieval tools

Prefer exact `note_slug` values from results when a tool asks for `note` or `seed`.

| Tool | Use |
| --- | --- |
| `notebrain_search` | Search with one or more `queries` or a `tag`; filter by section, content type, or exclusions |
| `notebrain_get` | Retrieve an indexed note; use `meta` for metadata or `head` for its first chunks |
| `notebrain_refs` | List attachments and external links in the current Markdown note under the configured vault |
| `notebrain_backlinks` | List incoming wikilinks to a note |
| `notebrain_connections` | Traverse links from a note with optional `hops` |
| `notebrain_hidden` | Find related notes; `deep` uses stored vectors without loading the model |
| `notebrain_tags` | List tags, search by tag, or find notes that share tags |
| `notebrain_boosted` | Search with a required `query` and graph `seed` |
| `notebrain_stats` | Show note, chunk, and link counts; takes no arguments |

Tool fields use snake_case. Omitted options use configured defaults or the CLI defaults. An explicit value replaces the configured value. For example, `exclude_notes: []` clears configured search exclusions. The tools do not expose ingestion, reset, setup, or arbitrary command execution.

Tool arguments and results have size and work limits. Each call accepts up to 32 queries, 256 exclusions, 16 KiB per string, and 64 KiB of effective input strings. Results larger than 4 MiB return an error, not incomplete JSON. Limits apply to effective configured options as well as explicit arguments. Valid ranges include `limit` and `head` 0 to 100, `top_k` and `hops` 1 to 3, `candidate_chunks` 1 to 10, `context_window` 0 to 5, `min_shared` 1 to 100, and `boost` 0 to 10. A zero limit keeps the corresponding CLI behavior.

## Concurrency and shared stores

The default limit is four active tool calls per server. Set `--max-concurrent-requests` to a value from 1 to 16. If all slots are occupied, the server returns a busy tool error without queuing a new call. Retry later. Discovery remains available. An admitted call has up to 90 seconds, including any wait for the store.

Each call starts a local NoteBrain child process, which adds startup overhead and isolates native failures from the protocol connection. A shared store lock lets only one process use the same embedded Chroma directory at a time. Concurrent tool calls can therefore wait behind each other or behind scheduled ingestion or reset. A timeout can indicate contention, not a damaged index.

Use an updated NoteBrain binary in every client and scheduled job that shares the store. Old binaries and direct Chroma clients do not use this advisory lock. The lock is intended for same-user Linux processes on a local filesystem. First use needs write access to the store's parent directory, where NoteBrain keeps a `.notebrain-store-*.lock` file. Do not delete the lock file while any process may use the store.

## Data access

The transport is local, but the client can send retrieved data to its model provider. Connect only clients you trust. Results can include note text, slugs, configured file paths, and reference URLs. Treat retrieved content as data, not instructions.

No tool fetches URLs or reads attachment bytes. Retrieval is not a filesystem read-only guarantee: opening an existing Chroma store can initialize collections or make other native writes.
