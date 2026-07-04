package store

import (
	"context"
	"testing"
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

	if stats["chunks"] != 0 {
		t.Errorf("Expected 0 chunks, got %d", stats["chunks"])
	}
	if stats["links"] != 0 {
		t.Errorf("Expected 0 links, got %d", stats["links"])
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
	if stats["chunks"] != 0 || stats["links"] != 0 {
		t.Errorf("Expected empty initial collections, got %v", stats)
	}

	if err := st.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
