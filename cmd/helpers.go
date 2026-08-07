package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// storeAPI is the subset of *store.Store used by commands. Commands depend on
// this narrow interface instead of the concrete store, so tests can inject
// fakes.
type storeAPI interface {
	Close() error
	ResolveNoteSlug(ctx context.Context, input string) (string, error)
	ResolveNoteSlugs(ctx context.Context, inputs []string) (map[string]string, map[string]struct{}, error)
	Backlinks(ctx context.Context, targetSlug string) ([]store.Result, error)
	Connections(ctx context.Context, seedSlug string, maxHops int) ([]store.Result, error)
	HiddenConnections(ctx context.Context, queryVec []float32, seedSlug string, limit int, includeText bool, options ...store.HiddenOption) ([]store.Result, error)
	HiddenConnectionsDeep(ctx context.Context, seedSlug string, limit int, topKPerNote int, includeText bool, options ...store.HiddenOption) ([]store.Result, []string, error)
	SharedTags(ctx context.Context, noteSlug string, minShared int) ([]store.Result, error)
	TagSearch(ctx context.Context, tag string, limit int, hierarchical bool, whereFilter store.WhereFilter, includeText bool) ([]store.Result, error)
	ListTags(ctx context.Context, limit int) ([]store.TagCount, error)
	SuggestTags(ctx context.Context, input string, limit int) ([]string, error)
	SemanticSearch(ctx context.Context, queryVec []float32, limit int, topKPerNote int, whereFilter store.WhereFilter, includeText bool) ([]store.Result, error)
	LexicalSearch(ctx context.Context, query string, limit int, whereFilter store.WhereFilter) ([]store.Result, error)
	MultiSemanticSearch(ctx context.Context, queryVecs [][]float32, queries []string, limit int, topKPerNote int, whereFilter store.WhereFilter, includeText bool) ([]store.Result, error)
	GraphBoostedSearch(ctx context.Context, queryVec []float32, seedSlug string, boost float64, limit int, whereFilter store.WhereFilter, includeText bool) ([]store.Result, error)
	GetNote(ctx context.Context, slugOrPath string) (*store.NoteContent, error)
	GetNoteMeta(ctx context.Context, slugOrPath string) (*store.NoteContent, error)
	GetNoteHead(ctx context.Context, slugOrPath string, maxChunks int) (*store.NoteContent, error)
	GetNoteMetadata(ctx context.Context) (map[string]store.NoteMeta, error)
	Stats(ctx context.Context) (*store.Stats, error)
	PopulateContext(ctx context.Context, results []store.Result, windowSize int) error
	Reset(ctx context.Context) error
}

// embedderAPI is the subset of *embedder.LocalEmbedder used by commands.
type embedderAPI interface {
	Embed(ctx context.Context, text string) ([]float32, error)
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	Close() error
}

// openStore opens the persistent ChromaDB store using global configuration.
// It is a package variable so tests can substitute a fake store.
var openStore = func(ctx context.Context, globals *Globals) (storeAPI, error) {
	return store.Open(ctx, globals.ChromaPath)
}

// formatTags joins tags as a comma-separated string.
func formatTags(tags []string) string {
	return strings.Join(tags, ",")
}

// formatTagChips renders tags as "#tag" chips for display.
func formatTagChips(tags []string) []string {
	chips := make([]string, 0, len(tags))
	for _, t := range tags {
		chips = append(chips, "#"+t)
	}
	return chips
}

// populateContext attaches surrounding-chunk context to results, wrapping
// store errors with the shared "populate context" prefix so every command
// reports context failures consistently.
func populateContext(ctx context.Context, st storeAPI, results []store.Result, windowSize int) error {
	if err := st.PopulateContext(ctx, results, windowSize); err != nil {
		return fmt.Errorf("populate context: %w", err)
	}
	return nil
}
