package store

import (
	"context"
	"fmt"
	"maps"
	"sync"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

const (
	CollectionChunks = "nb_chunks"
	CollectionLinks  = "nb_links"
)

// FileTypeMD and FileTypePDF are the file_type metadata values written on
// ingested chunks. Kept in store (not ingest) because store queries filter on
// them; ingest imports store, so there is no import cycle.
const (
	FileTypeMD  = "md"
	FileTypePDF = "pdf"
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

// collectionEmbeddingOptions are only used to satisfy Chroma's collection
// lifecycle. NoteBrain supplies explicit embeddings for every write and query,
// so opening a collection must not initialize or download the MiniLM model.
func collectionEmbeddingOptions() []chroma.CreateCollectionOption {
	dense := embeddings.NewConsistentHashEmbeddingFunction()
	content := embeddings.AdaptEmbeddingFunctionToContent(dense, embeddings.CapabilityMetadata{
		Modalities:    []embeddings.Modality{embeddings.ModalityText},
		SupportsBatch: true,
	})
	return []chroma.CreateCollectionOption{
		chroma.WithEmbeddingFunctionCreate(dense),
		chroma.WithContentEmbeddingFunctionCreate(content),
	}
}

func cloneMetaMap(m map[string]any) map[string]any {
	c := make(map[string]any, len(m))
	maps.Copy(c, m)
	return c
}

// Store wraps two ChromaDB collections.
type Store struct {
	client          chroma.Client
	chunks          chroma.Collection
	links           chroma.Collection
	mu              sync.RWMutex
	SkipAttachments bool

	// linkResolver is a lazily built link-target resolution table, reused
	// across commands within one Store lifetime (see linkResolverLocked).
	// All access is guarded by mu.
	linkResolver      map[string]string
	linkResolverValid bool
}

// Option configures Store when calling Open.
type Option func(*Store)

// Open creates or opens the persistent ChromaDB store at path.
func Open(ctx context.Context, path string, opts ...Option) (*Store, error) {
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
		options := append([]chroma.CreateCollectionOption{
			chroma.WithCollectionMetadataMapCreateStrict(chunksMeta),
		}, collectionEmbeddingOptions()...)
		chunks, err = client.GetOrCreateCollection(ctx, CollectionChunks, options...)
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
		options := append([]chroma.CreateCollectionOption{
			chroma.WithCollectionMetadataMapCreateStrict(linksMeta),
		}, collectionEmbeddingOptions()...)
		links, err = client.GetOrCreateCollection(ctx, CollectionLinks, options...)
		if err == nil {
			_, _ = links.Count(ctx) // Force lazy-loading of HNSW index under suppressor
		}
	})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("get/create links collection: %w", err)
	}

	st := &Store{client: client, chunks: chunks, links: links, SkipAttachments: true}
	for _, opt := range opts {
		opt(st)
	}
	return st, nil
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
	chunkOptions := append([]chroma.CreateCollectionOption{
		chroma.WithCollectionMetadataMapCreateStrict(cloneMetaMap(defaultChunksMeta)),
	}, collectionEmbeddingOptions()...)
	s.chunks, err = s.client.GetOrCreateCollection(ctx, CollectionChunks, chunkOptions...)
	if err != nil {
		return fmt.Errorf("recreate chunks collection: %w", err)
	}

	linkOptions := append([]chroma.CreateCollectionOption{
		chroma.WithCollectionMetadataMapCreateStrict(cloneMetaMap(defaultLinksMeta)),
	}, collectionEmbeddingOptions()...)
	s.links, err = s.client.GetOrCreateCollection(ctx, CollectionLinks, linkOptions...)
	if err != nil {
		return fmt.Errorf("recreate links collection: %w", err)
	}
	s.linkResolverValid = false
	return nil
}

// Stats reports collection counts. The typed struct (rather than a
// string-keyed map) keeps cmd/stats.go and JSON output safe from typos.
type Stats struct {
	Notes  int64 `json:"notes"`
	Chunks int64 `json:"chunks"`
	Links  int64 `json:"links"`
}

// Stats returns document counts for collections and distinct notes.
func (s *Store) Stats(ctx context.Context) (*Stats, error) {
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
	var distinctNotes int64
	if nc > 0 {
		seen := make(map[string]struct{})
		metas, err := s.paginatedZeroIndexMetadatas(ctx)
		if err != nil {
			return nil, fmt.Errorf("stats distinct notes: %w", wrapChromaErr(err))
		}
		for _, m := range metas {
			if slug, ok := m.GetString("note_slug"); ok && slug != "" {
				seen[slug] = struct{}{}
			}
		}
		// Fallback in case chunk_index=0 filter didn't match anything (e.g. older index format)
		if len(seen) == 0 {
			metas, err = paginatedGetMetadatas(ctx, s.chunks, nil)
			if err != nil {
				return nil, fmt.Errorf("stats distinct notes (fallback): %w", wrapChromaErr(err))
			}
			for _, m := range metas {
				if slug, ok := m.GetString("note_slug"); ok && slug != "" {
					seen[slug] = struct{}{}
				}
			}
		}
		distinctNotes = int64(len(seen))
	}
	return &Stats{Chunks: int64(nc), Links: int64(nl), Notes: distinctNotes}, nil
}
