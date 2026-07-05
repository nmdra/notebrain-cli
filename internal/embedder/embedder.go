package embedder

import (
	"context"
	"fmt"

	"github.com/amikos-tech/chroma-go/pkg/embeddings/ort"
	"github.com/nmdra/notebrain-cli/v2/internal/tui"
)

type LocalEmbedder struct {
	ef      *ort.DefaultEmbeddingFunction
	destroy func() error
}

func NewLocalEmbedder() (*LocalEmbedder, error) {
	var ef *ort.DefaultEmbeddingFunction
	var destroy func() error
	var err error

	done := make(chan struct{})
	go func() {
		defer close(done)
		ef, destroy, err = ort.NewDefaultEmbeddingFunction()
	}()

	if pErr := tui.RunSpinner("Setting up MiniLM (first run downloads ~33 MB)...", done); pErr != nil {
		return nil, fmt.Errorf("spinner error: %w", pErr)
	}

	if err != nil {
		return nil, fmt.Errorf("init local embedder: %w", err)
	}
	return &LocalEmbedder{ef: ef, destroy: destroy}, nil
}

func (e *LocalEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	batch, err := e.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return batch[0], nil
}

func (e *LocalEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	// Filter out empty strings which cause ONNX errors
	cleanTexts := make([]string, len(texts))
	for i, t := range texts {
		if t == "" {
			cleanTexts[i] = " "
		} else {
			cleanTexts[i] = t
		}
	}

	embs, err := e.ef.EmbedDocuments(ctx, cleanTexts)
	if err != nil {
		return nil, err
	}
	out := make([][]float32, len(embs))
	for i, emb := range embs {
		out[i] = emb.ContentAsFloat32()
	}
	return out, nil
}

func (e *LocalEmbedder) Close() error {
	if e.destroy != nil {
		return e.destroy()
	}
	return nil
}
