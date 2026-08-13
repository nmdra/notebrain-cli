/*
Copyright © 2026 nmdra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"errors"
	"log/slog"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/ingest"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type IngestCmd struct {
	Glob             string `arg:"" optional:"" help:"glob pattern to filter files (default: all .md files)"`
	Workers          int    `group:"ingest" help:"number of concurrent ingestion workers" default:"4"`
	MinChunkWords    int    `group:"ingest" name:"min-chunk-words" help:"skip chunks with fewer words" default:"10"`
	ChunkSize        int    `group:"ingest" name:"chunk-size" help:"max runes per chunk" default:"800"`
	ChunkOverlap     int    `group:"ingest" name:"chunk-overlap" help:"overlap runes between sub-chunks" default:"100"`
	RespectExclude   bool   `group:"ingest" help:"respect Obsidian userIgnoreFilters and attachmentFolderPath settings during ingest" default:"false"`
	WithPDF          bool   `group:"ingest" name:"with-pdf" help:"include PDF attachments in indexing" default:"false"`
	EnablePDF        bool   `group:"ingest" name:"enable-pdf" hidden:"" help:"deprecated: use --with-pdf" default:"false"`
	LLMModel         string `group:"ingest" name:"llm-model" help:"LLM model to use for PDF parsing (e.g. openrouter/anthropic/claude-sonnet, deepseek-chat). Requires API key in env." default:"" completion-predictor:"llm-model"`
	LLMContextWindow int    `group:"ingest" name:"llm-context-window" help:"total context window size of the LLM in tokens. Set this to match your specific model." default:"128000"`
}

func (c *IngestCmd) Run(globals *Globals) error {
	workers := c.Workers
	vaultPath := globals.VaultPath
	if vaultPath == "" {
		return &UsageError{Err: errors.New(vaultPathUsageError)}
	}

	glob := c.Glob

	chromaPath := globals.ChromaPath
	ctx := globals.Ctx

	slog.Info("opening vector store", "chroma_path", chromaPath)
	st, err := store.Open(ctx, chromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	slog.Info("initializing embedded ONNX vector models")
	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	slog.Info("starting ingestion pipeline", "workers", workers, "vault_path", vaultPath)
	pipeline := ingest.NewPipeline(st, emb, workers)

	pipeline.RespectExclude = c.RespectExclude
	pipeline.EnablePDF = c.WithPDF || c.EnablePDF
	pipeline.LLMModel = c.LLMModel
	pipeline.LLMContextWindow = c.LLMContextWindow
	pipeline.MinChunkWords = c.MinChunkWords
	pipeline.ChunkSize = c.ChunkSize
	pipeline.ChunkOverlap = c.ChunkOverlap
	return pipeline.Run(ctx, vaultPath, glob)
}
