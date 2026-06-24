package store

import (
	"context"
	"fmt"
	"sync"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

const (
	CollectionChunks = "nb_chunks"
	CollectionLinks  = "nb_links"
)

// Store wraps two ChromaDB collections.
type Store struct {
	client chroma.Client
	chunks chroma.Collection
	links  chroma.Collection
	mu     sync.Mutex
}

// Open creates or opens the persistent ChromaDB store at path.
func Open(ctx context.Context, path string) (*Store, error) {
	client, err := chroma.NewPersistentClient(
		chroma.WithPersistentPath(path),
		chroma.WithPersistentAllowReset(true),
		chroma.WithPersistentClientOption(
			chroma.WithDatabaseAndTenant("default_database", "default_tenant"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("chroma open %s: %w", path, err)
	}

	// Tune HNSW index for faster querying and proper metric
	meta := map[string]interface{}{
		"hnsw:space":       "cosine", // MiniLM embeddings are cosine-optimized
		"hnsw:search_ef":   50,       // Lower value improves query speed (default is 100)
		"hnsw:num_threads": 1,        // Prevent hnswlib background thread crash under load
	}

	chunks, err := client.GetOrCreateCollection(ctx, CollectionChunks, chroma.WithCollectionMetadataMapCreateStrict(meta))
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("get/create chunks collection: %w", err)
	}

	links, err := client.GetOrCreateCollection(ctx, CollectionLinks, chroma.WithCollectionMetadataMapCreateStrict(meta))
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("get/create links collection: %w", err)
	}

	return &Store{client: client, chunks: chunks, links: links}, nil
}

// Close releases all resources.
func (s *Store) Close() error {
	return s.client.Close()
}

// Reset drops and recreates both collections. Used by `notebrain reset`.
func (s *Store) Reset(ctx context.Context) error {
	for _, name := range []string{CollectionChunks, CollectionLinks} {
		if err := s.client.DeleteCollection(ctx, name); err != nil {
			return fmt.Errorf("delete collection %s: %w", name, err)
		}
	}
	var err error
	s.chunks, err = s.client.GetOrCreateCollection(ctx, CollectionChunks)
	if err != nil {
		return err
	}
	s.links, err = s.client.GetOrCreateCollection(ctx, CollectionLinks)
	return err
}

// Stats returns document counts for both collections.
func (s *Store) Stats(ctx context.Context) (map[string]int64, error) {
	nc, err := s.chunks.Count(ctx)
	if err != nil {
		return nil, err
	}
	nl, err := s.links.Count(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]int64{
		"chunks": int64(nc),
		"links":  int64(nl),
	}, nil
}
