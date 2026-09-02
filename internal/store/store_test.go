package store

import (
	"context"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

func TestStoreOpenClose(t *testing.T) {
	ctx := context.Background()

	// Open store with temp dir
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	// Initial stats should be empty
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Chunks != 0 {
		t.Errorf("Expected 0 chunks, got %d", stats.Chunks)
	}
	if stats.Links != 0 {
		t.Errorf("Expected 0 links, got %d", stats.Links)
	}
	if stats.Notes != 0 {
		t.Errorf("Expected 0 notes, got %d", stats.Notes)
	}
}

func TestStoreOpen_UsesNonDownloadingEmbeddingFunction(t *testing.T) {
	st, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Open failed without ONNX model: %v", err)
	}
	defer func() { _ = st.Close() }()

	for _, collection := range []struct {
		name string
		col  chroma.Collection
	}{
		{name: CollectionChunks, col: st.chunks},
		{name: CollectionLinks, col: st.links},
	} {
		raw, ok := collection.col.Configuration().GetRaw("embedding_function")
		if !ok {
			t.Fatalf("%s collection has no embedding function configuration", collection.name)
		}
		config, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("%s embedding function configuration has type %T", collection.name, raw)
		}
		if got, _ := config["name"].(string); got != "consistent_hash" {
			t.Errorf("%s embedding function = %q, want consistent_hash", collection.name, got)
		}
	}
}

// defaultNamedEmbeddingFunction writes a persisted "default" embedding
// function without constructing Chroma's download-backed implementation.
type defaultNamedEmbeddingFunction struct{}

func (defaultNamedEmbeddingFunction) EmbedDocuments(context.Context, []string) ([]embeddings.Embedding, error) {
	return nil, nil
}

func (defaultNamedEmbeddingFunction) EmbedQuery(context.Context, string) (embeddings.Embedding, error) {
	return nil, nil
}

func (defaultNamedEmbeddingFunction) Name() string { return "default" }

func (defaultNamedEmbeddingFunction) GetConfig() embeddings.EmbeddingFunctionConfig {
	return embeddings.EmbeddingFunctionConfig{}
}

func (defaultNamedEmbeddingFunction) DefaultSpace() embeddings.DistanceMetric {
	return embeddings.COSINE
}

func (defaultNamedEmbeddingFunction) SupportedSpaces() []embeddings.DistanceMetric {
	return []embeddings.DistanceMetric{embeddings.COSINE}
}

func TestStoreOpen_ReopensPersistedDefaultEmbeddingFunction(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()

	client, err := chroma.NewPersistentClient(
		chroma.WithPersistentPath(path),
		chroma.WithPersistentAllowReset(true),
		chroma.WithPersistentClientOption(
			chroma.WithDatabaseAndTenant("default_database", "default_tenant"),
		),
	)
	if err != nil {
		t.Fatalf("create persistent client: %v", err)
	}
	_, err = client.CreateCollection(ctx, CollectionChunks,
		chroma.WithCollectionMetadataMapCreateStrict(cloneMetaMap(defaultChunksMeta)),
		chroma.WithEmbeddingFunctionCreate(defaultNamedEmbeddingFunction{}),
	)
	if err != nil {
		_ = client.Close()
		t.Fatalf("create default embedding collection: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close seed client: %v", err)
	}

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open failed when persisted default embedding function was present: %v", err)
	}
	defer func() { _ = st.Close() }()

	raw, ok := st.chunks.Configuration().GetRaw("embedding_function")
	if !ok {
		t.Fatal("reopened collection has no embedding function configuration")
	}
	config, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("reopened embedding function configuration has type %T", raw)
	}
	if got, _ := config["name"].(string); got != "default" {
		t.Errorf("reopened embedding function = %q, want persisted default", got)
	}
}

func TestStats_UniqueNotesCount(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	chunks := []ChunkRecord{
		{
			ID:         "note-a:0",
			NoteSlug:   "note-a",
			Title:      "Note A",
			ChunkIndex: 0,
			Text:       "First chunk of A",
			Embedding:  []float32{0.1, 0.2, 0.3},
		},
		{
			ID:         "note-a:1",
			NoteSlug:   "note-a",
			Title:      "Note A",
			ChunkIndex: 1,
			Text:       "Second chunk of A",
			Embedding:  []float32{0.2, 0.3, 0.4},
		},
		{
			ID:         "note-b:0",
			NoteSlug:   "note-b",
			Title:      "Note B",
			ChunkIndex: 0,
			Text:       "First chunk of B",
			Embedding:  []float32{0.3, 0.4, 0.5},
		},
	}

	bySlug := make(map[string][]ChunkRecord)
	for _, c := range chunks {
		bySlug[c.NoteSlug] = append(bySlug[c.NoteSlug], c)
	}
	data := make([]BatchIngestData, 0, len(bySlug))
	for slug, recs := range bySlug {
		data = append(data, BatchIngestData{NoteSlug: slug, ChunkRecords: recs})
	}
	if err := st.BatchIngest(ctx, data, nil); err != nil {
		t.Fatalf("BatchIngest failed: %v", err)
	}

	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.Chunks != 3 {
		t.Errorf("Expected 3 chunks, got %d", stats.Chunks)
	}
	if stats.Notes != 2 {
		t.Errorf("Expected 2 distinct notes, got %d", stats.Notes)
	}
}

func TestStoreReset(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = st.Close() }()

	err = st.Reset(ctx)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
}

func TestStoreOpen_StrictPersistentOnly(t *testing.T) {
	ctx := context.Background()
	path := t.TempDir()

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	if st.client == nil {
		t.Fatal("Expected persistent client to be non-nil")
	}
	if st.chunks == nil {
		t.Fatal("Expected chunks collection to be initialized")
	}
	if st.links == nil {
		t.Fatal("Expected links collection to be initialized")
	}

	// Verify stats work without network/HTTP server
	stats, err := st.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Chunks != 0 || stats.Links != 0 {
		t.Errorf("Expected empty initial collections, got %v", stats)
	}

	if err := st.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
