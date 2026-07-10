package store_test

import (
	"context"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// newTestStore opens a fresh temporary store for testing and registers cleanup.
func newTestStore(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
