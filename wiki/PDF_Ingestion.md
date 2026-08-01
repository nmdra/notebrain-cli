# PDF Ingestion with LLMs

NoteBrain indexes your PDF attachments with Large Language Models (LLMs). The LLMs parse text and structural metadata. Semantic search then works on your documents, research papers, and ebooks.

## Requirements

1. **LLM API Key**

## Supported LLM Providers

NoteBrain detects the provider automatically. It uses the model prefix or the available API keys in your environment.

| Provider           | Environment Variable | Model Syntax Example                  |
| :----------------- | :------------------- | :------------------------------------ |
| **OpenRouter**     | `OPENROUTER_API_KEY` | `--llm-model="tencent/hy3"`           |
| **DeepSeek**       | `DEEPSEEK_API_KEY`   | `--llm-model="deepseek-v4-flash"`     |
| **OpenAI**         | `OPENAI_API_KEY`     | `--llm-model="gpt-4o"`                |
| **Gemini**         | `GEMINI_API_KEY`     | `--llm-model="gemini-3.5-flash-lite"` |
| **Ollama** (Local) | `OLLAMA_HOST`        | `--llm-model="ollama/llama3"`         |

## How to Enable

To index PDFs during an ingestion run, provide the `--enable-pdf` and `--llm-model` flags:

```bash
export OPENROUTER_API_KEY="your-key-here"

notebrain ingest --enable-pdf --llm-model="tencent/hy3"
```

You can set these values permanently with the CLI wizard:

```bash
notebrain init
```

Alternatively, put them in your `~/.notebrain/config.toml`:

```toml
enable_pdf = true
llm_model = "tencent/hy3"
```

## Fallbacks and Cost Control

If you run `notebrain ingest` with `--enable-pdf` but your API key is missing, NoteBrain has a fallback process:

- NoteBrain prints a warning that PDF ingestion is disabled.
- NoteBrain skips new or updated PDFs.
- NoteBrain keeps previously ingested PDFs in the ChromaDB index.
- NoteBrain continues to ingest your standard Markdown (`.md`) files.

Because of this, you can schedule regular ingestion runs in the background (for example, with `cron`). If your environment variables are incorrect, you do not lose PDF data.

## Search PDFs

PDF search results do not show by default. To include PDF contents when you search your vault, use the `--with-pdf` flag:

```bash
notebrain search "machine learning fundamentals" --with-pdf
```
