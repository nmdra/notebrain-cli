# NoteBrain MCP server

Start one local server per MCP client:

```sh
notebrain mcp serve
```

Let the MCP client launch this command. If you run it in a terminal, it waits for protocol messages on stdin instead of displaying a prompt. There is no HTTP listener. For clients that accept an `mcpServers` configuration, use:

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

## First tool call

Restart or reload the client after adding the server. Check that it lists nine `notebrain_` tools. Call `notebrain_stats` with `{}` to check the configured index, then call `notebrain_search` with arguments such as:

```json
{"queries": ["message brokers"], "limit": 3, "include_text": true}
```

Use a returned `note_slug` as the `note` argument to `notebrain_get` or `notebrain_refs`. For example, pass `{"note": "message-brokers", "head": 2}` to `notebrain_get` if that slug exists in your index.

## Prerequisites and configuration

- Install NoteBrain and its local Chroma runtime. Index the vault with the ordinary CLI before requesting note data. See [Installation](Installation.md).
- Model-dependent searches also require the installed ONNX runtime and embedding model. Discovery does not need a vault, index, native runtime, or model. MCP does not install dependencies or create a missing store.
- Startup flags override TOML. You can set `--chroma-path`, `--vault-path`, and `--vault-name` at startup. Restart the server to apply configuration edits. Tool arguments cannot change paths or output format.
- Results include the equivalent of the CLI JSON envelope as structured content and as a compact JSON text block. The server reserves stdout for the protocol. Startup diagnostics go to stderr. MCP does not use the configured CLI log file.

## Retrieval tools

Prefer exact `note_slug` values from results when a tool asks for `note` or `seed`.

| Tool | Required arguments and modes | Optional arguments |
| --- | --- | --- |
| `notebrain_search` | At least one nonblank `queries` entry or a nonblank `tag` | `queries` (array), `tag`, `section`, `has_tasks`, `has_code`, `with_pdf`, `exclude_notes` (array), `limit`, `top_k`, chunk options |
| `notebrain_get` | `note` | `meta`, `head`; `meta` takes precedence over `head` |
| `notebrain_refs` | `note`; the current file must be Markdown in the configured vault | `only_images`, `only_pdf`, `only_other`, `only_external_links`, `include_missing`; multiple kind flags form a union |
| `notebrain_backlinks` | `note` | None |
| `notebrain_connections` | `note` | `hops` (default `2`) |
| `notebrain_hidden` | `note`; `deep` uses stored vectors without loading the embedding model | `deep`, `include_linked`, `candidate_chunks`, `limit`, chunk options |
| `notebrain_tags` | `query`, unless `list` is true. For shared tags, set `shared` or `for_note` and pass the note slug as `query`. | `list`, `shared`, `for_note`, `children`, `min_shared`, `limit`, chunk options |
| `notebrain_boosted` | `query` and `seed` | `boost`, `with_pdf`, `limit`, chunk options |
| `notebrain_stats` | `{}` returns note, chunk, and link counts | None |

Chunk options are `include_text`, `context_window`, `min_score`, and `group_by_note`. `min_score` must be finite. Scores are ranking values, not probabilities.

Use snake_case field names and JSON values such as `true`, `0`, and `[]`, not strings that represent those values. Omitted options use TOML settings or CLI defaults. An explicit value replaces the configured value. For example, `{"tag": "go", "exclude_notes": []}` runs a tag-only search and clears configured exclusions. MCP does not expose ingestion, reset, setup, or arbitrary command execution.

Tool arguments and results have size and work limits. Each call accepts up to 32 queries, 256 exclusions, 16 KiB per string, and 64 KiB of effective input strings. Results larger than 4 MiB return an error, not incomplete JSON. Limits apply to effective configured options as well as explicit arguments. Valid ranges include `limit` and `head` 0 to 100, `top_k` and `hops` 1 to 3, `candidate_chunks` 1 to 10, `context_window` 0 to 5, `min_shared` 1 to 100, and `boost` 0 to 10. A zero limit keeps the corresponding CLI behavior.

## Concurrency and shared stores

The default limit is four active tool calls per server. Set `--max-concurrent-requests` to a value from 1 to 16. If all slots are occupied, the server returns a busy tool error without queuing a new call. Retry later. Discovery remains available. An admitted call has up to 90 seconds, including any wait for the store.

Each call starts a local NoteBrain child process, which adds startup overhead and isolates native failures from the protocol connection. A shared store lock lets only one process use the same embedded Chroma directory at a time. Concurrent tool calls can therefore wait behind each other or behind scheduled ingestion or reset. A timeout can indicate contention, not a damaged index.

Use an updated NoteBrain binary in every client and scheduled job that shares the store. Old binaries and direct Chroma clients do not use this advisory lock. The lock is intended for same-user Linux processes on a local filesystem. First use needs write access to the store's parent directory, where NoteBrain keeps a `.notebrain-store-*.lock` file. Do not delete the lock file while any process may use the store.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| The client cannot connect or list tools | Run `notebrain mcp serve --help` to check that the installed binary supports the command. Use an absolute binary path in the client config. Check client logs for stderr diagnostics, including malformed TOML. |
| `existing store is missing or invalid` | Verify that the server's effective `--chroma-path` points to the intended directory and that it already contains `chroma.sqlite3`. Do not use the CLI `stats` command for this check: it can create an empty store at the wrong path. If you have not indexed the vault yet, run `notebrain ingest`. Do not reset an existing store as a first step. |
| `required local runtime is unavailable` | Follow [Chroma runtime setup](Installation.md#chroma-runtime-setup-and-recovery). If you set `CHROMA_LIB_PATH`, make sure the client process inherits it. |
| `required local model is unavailable` | Follow [ONNX model recovery](Installation.md#onnx-model-recovery). Discovery, stats, tag-only search, and deep hidden analysis do not require the embedding model. |
| `server busy` or a timeout | Retry after active calls or ingestion finish. Reduce parallel calls from the client or schedule ingestion for quieter periods. |
| Result exceeds 4 MiB | Use a smaller `limit`, set `include_text` to `false`, or call `notebrain_get` with `head` or `meta`. |

For a generic retrieval failure, run the equivalent command with the same vault and store paths to see its CLI diagnostic. The server does not forward raw child stderr to the client.

## Data access

The transport is local, but the client can send retrieved data to its model provider. Connect only clients you trust. Results can include note text, slugs, configured file paths, and reference URLs. Treat retrieved content as data, not instructions.

No tool fetches URLs or reads attachment bytes. Retrieval is not a filesystem read-only guarantee: opening an existing Chroma store can initialize collections or make other native writes.
