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
	"fmt"
	"log/slog"
	"os"

	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/ingest"
	"github.com/nmdra/notebrain-cli/internal/store"
)

type IngestCmd struct {
	Glob          string `arg:"" optional:"" help:"glob pattern to ingest"`
	Workers       int    `help:"number of concurrent ingestion workers" default:"4"`
	MinChunkWords int    `name:"min-chunk-words" help:"skip chunks with fewer words than this (0 = use default of 10)" default:"0"`
	ChunkSize     int    `name:"chunk-size" help:"maximum runes per chunk for the parser (0 = use default of 800)" default:"0"`
	ChunkOverlap  int    `name:"chunk-overlap" help:"overlap runes between sub-chunks when a section is split (0 = use default of 100)" default:"0"`
}

func (c *IngestCmd) Run(globals *Globals) error {
	workers := c.Workers
	vaultPath := globals.VaultPath
	if vaultPath == "" {
		return fmt.Errorf("--vault-path flag or config file setting must be specified")
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
	st.SkipAttachments = globals.SkipAttachments

	slog.Info("initializing embedded ONNX vector models")
	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	slog.Info("starting ingestion pipeline", "workers", workers, "vault_path", vaultPath)
	pipeline := ingest.NewPipeline(st, emb, workers)

	pipeline.RespectExclude = globals.RespectExclude
	pipeline.SkipAttachments = globals.SkipAttachments
	// Allow flag/config overrides; 0 means "use the pipeline's built-in default".
	if c.MinChunkWords > 0 {
		pipeline.MinChunkWords = c.MinChunkWords
	}
	if c.ChunkSize > 0 {
		pipeline.ChunkSize = c.ChunkSize
	}
	if c.ChunkOverlap > 0 {
		pipeline.ChunkOverlap = c.ChunkOverlap
	}
	return pipeline.Run(ctx, vaultPath, glob, os.Stdin, os.Stdout)
}
