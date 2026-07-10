package cmd

import (
	"context"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// openStore opens the persistent ChromaDB store using global configuration.
func openStore(ctx context.Context, globals *Globals) (*store.Store, error) {
	return store.Open(ctx, globals.ChromaPath, store.WithSkipAttachments(globals.SkipAttachments))
}

// formatTags joins tags as a comma-separated string.
func formatTags(tags []string) string {
	return strings.Join(tags, ",")
}
