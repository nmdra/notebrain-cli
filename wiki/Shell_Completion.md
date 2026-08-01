# Shell Completion

NoteBrain provides tab completion for **bash**, **zsh**, and **fish**. It completes subcommands, flags, enum values (for example `--format text|json|tsv`), and note slugs from your live index.

## Activation

Run `notebrain completion` to see the exact activation command for your shell:

```bash
notebrain completion bash
notebrain completion zsh
notebrain completion fish
```

The command prints a line to execute in your current session and tells you which init file to use for permanent activation:

- bash: add `source <(notebrain completion -c bash)` to `~/.bashrc`
- zsh: add `source <(notebrain completion -c zsh)` to `~/.zshrc`
- fish: add `notebrain completion -c fish | source` to `~/.config/fish/config.fish`

Without a shell argument, `notebrain completion` detects your login shell automatically.

## What Gets Completed

| Scope | Examples |
| --- | --- |
| Subcommands | `ingest`, `search`, `backlinks`, `connections`, `hidden`, `tags`, `boosted`, `stats`, `get`, `reset`, `doctor`, `init`, `version`, `completion` |
| Global flags | `--vault-path`, `--chroma-path`, `--format`, `--log-level`, `--limit`, `--top-k`, `--seed`, `--hops`, ... |
| Enum values | `--format` → `text`, `json`, `tsv`; `--log-level` → `debug`, `info`, `warn`, `error` |
| `--llm-model` | Documented model values (`openrouter/anthropic/claude-3.5-haiku`, `deepseek-chat`, `gemini-3.5-flash-lite`, `ollama/<model>`) |
| Note slugs (dynamic) | Positional `<note>` args of `get`, `backlinks`, `connections`, `hidden`, and `tags`, plus `--seed` of `boosted` |

## Note Slug Completion

Slug candidates come from your live ChromaDB index. When you press Tab after a `<note>` argument, NoteBrain lists the indexed note slugs.

- Requires an indexed database. Run `notebrain ingest` first; an empty database completes nothing.
- Slugs are fetched by the hidden `suggest-notes` command (one process spawn per Tab press). It is a paginated metadata scan with no embeddings or LLM calls, so it is fast.

## Limitations

- Completions only work for a binary named `notebrain` in your `$PATH`. `./notebrain` (relative invocation) is not completed.
- Slug candidates come from the config or default chroma path (`~/.notebrain/chroma`). A `--chroma-path` flag typed on the command line is not reflected in completions.
- The hidden `suggest-notes` command does not appear in completion lists or `notebrain --help`.
- Vault subfolder paths and tag names are not completed dynamically.

## Troubleshooting

- **No completions after adding the line**: restart the shell or run `exec bash` / `exec zsh` so the init file is re-sourced.
- **Slug completion is empty**: confirm the index is populated with `notebrain stats`.
- **Completions appear twice or are stale**: make sure only one activation line exists in your init file.
