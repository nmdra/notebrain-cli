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
	defer st.Close()

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
	defer st.Close()

	err = st.Reset(ctx)
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}
}
