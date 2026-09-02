package embedder_test

import (
	"context"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
)

func TestLocalEmbedder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping embedder test in short mode")
	}
	if err := embedder.CheckModelAvailability(); err != nil {
		t.Skipf("skipping embedder integration test: %v", err)
	}

	emb, err := embedder.NewLocalEmbedder()
	if err != nil {
		t.Fatalf("NewLocalEmbedder failed: %v", err)
	}
	defer func() {
		if err := emb.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
	}()

	ctx := context.Background()

	t.Run("Model", func(t *testing.T) {
		if got := emb.Model(); got == "" {
			t.Errorf("expected non-empty model identifier")
		}
	})

	t.Run("EmbedSingle", func(t *testing.T) {
		vec, err := emb.Embed(ctx, "hello world")
		if err != nil {
			t.Fatalf("Embed failed: %v", err)
		}
		if len(vec) == 0 {
			t.Errorf("expected non-empty vector")
		}
	})

	t.Run("EmbedBatch", func(t *testing.T) {
		tests := []struct {
			name    string
			input   []string
			wantErr bool
		}{
			{
				name:    "normal strings",
				input:   []string{"golang", "vector search", "chromadb"},
				wantErr: false,
			},
			{
				name:    "contains empty string",
				input:   []string{"first", "", "third"},
				wantErr: false,
			},
			{
				name:    "all empty strings",
				input:   []string{"", ""},
				wantErr: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				vecs, err := emb.EmbedBatch(ctx, tt.input)
				if (err != nil) != tt.wantErr {
					t.Fatalf("EmbedBatch() error = %v, wantErr %v", err, tt.wantErr)
				}
				if !tt.wantErr {
					if len(vecs) != len(tt.input) {
						t.Errorf("expected %d vectors, got %d", len(tt.input), len(vecs))
					}
					for i, v := range vecs {
						if len(v) == 0 {
							t.Errorf("vector %d is empty", i)
						}
					}
				}
			})
		}
	})
}
