package cmd

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// fakeStore is a minimal in-memory storeAPI for command tests. It records
// mutation calls and returns zero values for queries; tests can extend it
// with the behavior they need.
type fakeStore struct {
	mu          sync.Mutex
	resetCalls  int
	resetErr    error
	tags        []store.TagCount
	suggest     []string
	semantic    []store.Result
	lexical     []store.Result
	metaCalls   int
	headCalls   int
	noteMeta    *store.NoteContent
	noteMetaErr error
	noteHead    *store.NoteContent
	stats       *store.Stats
}

func (f *fakeStore) Close() error { return nil }

func (f *fakeStore) ResolveNoteSlug(_ context.Context, input string) (string, error) {
	return input, nil
}

func (f *fakeStore) ResolveNoteSlugs(context.Context, []string) (map[string]string, map[string]struct{}, error) {
	return nil, nil, nil
}

func (f *fakeStore) Backlinks(context.Context, string) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) Connections(context.Context, string, int) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) HiddenConnections(context.Context, []float32, string, int, bool, ...store.HiddenOption) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) HiddenConnectionsDeep(context.Context, string, int, int, bool, ...store.HiddenOption) ([]store.Result, []string, error) {
	return nil, nil, nil
}

func (f *fakeStore) SharedTags(context.Context, string, int) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) TagSearch(context.Context, string, int, bool, store.WhereFilter, bool) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) ListTags(_ context.Context, limit int) ([]store.TagCount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit > 0 && len(f.tags) > limit {
		return append([]store.TagCount(nil), f.tags[:limit]...), nil
	}
	return f.tags, nil
}

func (f *fakeStore) SuggestTags(context.Context, string, int) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.suggest, nil
}

func (f *fakeStore) SemanticSearch(context.Context, []float32, int, int, store.WhereFilter, bool) ([]store.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.semantic, nil
}

func (f *fakeStore) LexicalSearch(context.Context, string, int, store.WhereFilter) ([]store.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lexical, nil
}

func (f *fakeStore) MultiSemanticSearch(context.Context, [][]float32, []string, int, int, store.WhereFilter, bool) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) GraphBoostedSearch(context.Context, []float32, string, float64, int, store.WhereFilter, bool) ([]store.Result, error) {
	return nil, nil
}

func (f *fakeStore) GetNote(context.Context, string) (*store.NoteContent, error) {
	return nil, nil
}

func (f *fakeStore) GetNoteMeta(context.Context, string) (*store.NoteContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metaCalls++
	if f.noteMetaErr != nil {
		return nil, f.noteMetaErr
	}
	return f.noteMeta, nil
}

func (f *fakeStore) GetNoteHead(context.Context, string, int) (*store.NoteContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headCalls++
	return f.noteHead, nil
}

func (f *fakeStore) GetNoteMetadata(context.Context) (map[string]store.NoteMeta, error) {
	return nil, nil
}

func (f *fakeStore) Stats(context.Context) (*store.Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stats, nil
}

func (f *fakeStore) PopulateContext(context.Context, []store.Result, int) error {
	return nil
}

func (f *fakeStore) Reset(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.resetCalls++
	return f.resetErr
}

// withFakeStore points the package openStore variable at fs for the
// duration of the test, so commands that open a store exercise the same
// code path without touching ChromaDB.
func withFakeStore(t *testing.T, fs storeAPI) {
	t.Helper()
	orig := openStore
	openStore = func(context.Context, *Globals) (storeAPI, error) {
		return fs, nil
	}
	t.Cleanup(func() { openStore = orig })
}

// withStdin replaces os.Stdin with a pipe preloaded with input, restored
// when the test finishes.
func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
		_ = w.Close()
	})
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
}
