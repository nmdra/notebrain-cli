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

var defaultChunksMeta = map[string]any{
	"hnsw:space":           "cosine",
	"hnsw:search_ef":       50, // Lower value improves query speed
	"hnsw:num_threads":     1,  // Prevent hnswlib background thread crash
	"hnsw:M":               32, // Prevent isolated nodes and HNSW integrity check assertion crashes
	"hnsw:construction_ef": 200,
}

var defaultLinksMeta = map[string]any{
	"hnsw:space":       "l2",
	"hnsw:num_threads": 1,
}

func cloneMetaMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// Store wraps two ChromaDB collections.
type Store struct {
	client          chroma.Client
	chunks          chroma.Collection
	links           chroma.Collection
	mu              sync.RWMutex
	SkipAttachments bool
}

// Open creates or opens the persistent ChromaDB store at path.
func Open(ctx context.Context, path string) (*Store, error) {
	var client chroma.Client
	var chunks chroma.Collection
	var links chroma.Collection
	var err error

	suppressOutputs(func() {
		client, err = chroma.NewPersistentClient(
			chroma.WithPersistentPath(path),
			chroma.WithPersistentAllowReset(true),
			chroma.WithPersistentClientOption(
				chroma.WithDatabaseAndTenant("default_database", "default_tenant"),
			),
		)
	})
	if err != nil {
		return nil, fmt.Errorf("chroma open %s: %w", path, err)
	}

	// Tune HNSW index for chunks (MiniLM embeddings are cosine-optimized)
	chunksMeta := cloneMetaMap(defaultChunksMeta)

	suppressOutputs(func() {
		chunks, err = client.GetOrCreateCollection(ctx, CollectionChunks, chroma.WithCollectionMetadataMapCreateStrict(chunksMeta))
		if err == nil {
			_, _ = chunks.Count(ctx) // Force lazy-loading of HNSW index under suppressor
		}
	})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("get/create chunks collection: %w", err)
	}

	// Tune HNSW index for links (uses dummy embeddings, L2 distance avoids cosine degeneracy)
	linksMeta := cloneMetaMap(defaultLinksMeta)

	suppressOutputs(func() {
		links, err = client.GetOrCreateCollection(ctx, CollectionLinks, chroma.WithCollectionMetadataMapCreateStrict(linksMeta))
		if err == nil {
			_, _ = links.Count(ctx) // Force lazy-loading of HNSW index under suppressor
		}
	})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("get/create links collection: %w", err)
	}

	return &Store{client: client, chunks: chunks, links: links, SkipAttachments: true}, nil
}

// Close releases all resources.
func (s *Store) Close() error {
	return s.client.Close()
}

// Reset drops and recreates both collections. Used by `notebrain reset`.
func (s *Store) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range []string{CollectionChunks, CollectionLinks} {
		if err := s.client.DeleteCollection(ctx, name); err != nil {
			return fmt.Errorf("delete collection %s: %w", name, err)
		}
	}

	var err error
	s.chunks, err = s.client.GetOrCreateCollection(ctx, CollectionChunks, chroma.WithCollectionMetadataMapCreateStrict(cloneMetaMap(defaultChunksMeta)))
	if err != nil {
		return fmt.Errorf("recreate chunks collection: %w", err)
	}

	s.links, err = s.client.GetOrCreateCollection(ctx, CollectionLinks, chroma.WithCollectionMetadataMapCreateStrict(cloneMetaMap(defaultLinksMeta)))
	if err != nil {
		return fmt.Errorf("recreate links collection: %w", err)
	}
	return nil
}

// Stats returns document counts for both collections.
func (s *Store) Stats(ctx context.Context) (map[string]int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	nc, err := s.chunks.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats chunks count: %w", err)
	}
	nl, err := s.links.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("stats links count: %w", err)
	}
	return map[string]int64{
		"chunks": int64(nc),
		"links":  int64(nl),
	}, nil
}
