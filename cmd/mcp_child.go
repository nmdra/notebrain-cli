package cmd

import (
	"errors"
	"fmt"

	"github.com/alecthomas/kong"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
	"github.com/nmdra/notebrain-cli/v2/internal/mcpbridge"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// MCPChildError carries a reserved exit status for an isolated retrieval child.
// The parent maps the status to a fixed diagnostic instead of reading stderr.
type MCPChildError struct {
	Err    error
	Status int
}

func (e *MCPChildError) Error() string { return e.Err.Error() }
func (e *MCPChildError) Unwrap() error { return e.Err }

func runMCPChild(parsed *kong.Context, globals *Globals, explicitConfig bool) error {
	selected := parsed.Selected()
	if !globals.MCPChild || !explicitConfig || selected == nil || !mcpRetrievalCommand(selected.Name) {
		err := fmt.Errorf("--mcp-child requires an explicit --config and a retrieval command")
		parsed.Errorf("%s", err)
		return &UsageError{Err: err}
	}
	globals.Format, globals.JSONPath = formatJSON, ""
	if err := store.CheckExistingDatabase(globals.Ctx, globals.ChromaPath); err != nil {
		return &MCPChildError{Err: fmt.Errorf("retrieval requires an existing Chroma database: %w", err), Status: mcpbridge.ExitStoreUnavailable}
	}
	if err := parsed.Run(globals); err != nil {
		return mcpChildFailure(err)
	}
	return nil
}

func mcpChildFailure(err error) error {
	switch {
	case errors.Is(err, store.ErrRuntimeUnavailable):
		return &MCPChildError{Err: fmt.Errorf("retrieval runtime: %w", err), Status: mcpbridge.ExitRuntimeUnavailable}
	case errors.Is(err, embedder.ErrModelUnavailable):
		return &MCPChildError{Err: fmt.Errorf("retrieval model: %w", err), Status: mcpbridge.ExitModelUnavailable}
	default:
		return err
	}
}
