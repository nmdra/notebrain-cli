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
	"os"

	"github.com/nmdra/notebrain-cli/internal/embedder"
	"github.com/nmdra/notebrain-cli/internal/ingest"
	"github.com/nmdra/notebrain-cli/internal/store"
)

type IngestCmd struct {
	Glob          string `arg:"" optional:"" help:"glob pattern to ingest"`
	Workers       int    `help:"number of concurrent ingestion workers" default:"4"`
	MinChunkWords int    `name:"min-chunk-words" help:"skip chunks containing fewer than this many words" default:"0"`
}

func (c *IngestCmd) Run(globals *Globals) error {
	workers := c.Workers
	vaultPath := globals.VaultPath
	if vaultPath == "" {
		vaultPath = os.Getenv("OBSIDIAN_VAULT_PATH")
	}
	if vaultPath == "" {
		return fmt.Errorf("--vault-path flag or OBSIDIAN_VAULT_PATH env var must be specified")
	}

	glob := c.Glob

	chromaPath := globals.ChromaPath
	ctx := globals.Ctx

	fmt.Println("Opening ChromaDB store...")
	st, err := store.Open(ctx, chromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	fmt.Println("Initializing embedded ONNX vector models...")
	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		return err
	}
	defer func() { _ = emb.Close() }()

	fmt.Printf("Starting ingestion pipeline with %d workers...\n", workers)
	pipeline := ingest.NewPipeline(st, emb, workers)
	pipeline.MinChunkWords = c.MinChunkWords
	return pipeline.Run(ctx, vaultPath, glob, os.Stdin, os.Stdout)
}
