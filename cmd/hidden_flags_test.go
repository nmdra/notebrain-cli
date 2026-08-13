package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// hiddenParser builds a kong parser bound to a HiddenCmd so flag-level
// behavior can be asserted without running the command.
func hiddenParser(t *testing.T) (*kong.Kong, *HiddenCmd) {
	t.Helper()
	var cli struct {
		Globals Globals   `embed:""`
		Hidden  HiddenCmd `cmd:""`
	}
	parser, err := kong.New(&cli, kong.ExplicitGroups(helpGroups()))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	return parser, &cli.Hidden
}

func TestHiddenTopKRemovedInFavorOfCandidateChunks(t *testing.T) {
	parser, hidden := hiddenParser(t)

	t.Run("candidate-chunks accepts value", func(t *testing.T) {
		if _, err := parser.Parse([]string{"hidden", "x", "--candidate-chunks", "5"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if hidden.CandidateChunks != 5 {
			t.Errorf("CandidateChunks = %d, want 5", hidden.CandidateChunks)
		}
	})

	t.Run("top-k is rejected", func(t *testing.T) {
		_, err := parser.Parse([]string{"hidden", "x", "--top-k", "5"})
		if err == nil {
			t.Errorf("expected --top-k to be rejected, got no error")
		}
		if err != nil && !strings.Contains(err.Error(), "unknown flag") {
			t.Errorf("expected unknown-flag error, got: %v", err)
		}
	})

	var help strings.Builder
	parser.Stdout = &help
	ctx, err := parser.Parse([]string{"hidden", "x"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatalf("PrintUsage: %v", err)
	}
	if !strings.Contains(help.String(), "--candidate-chunks") {
		t.Errorf("help must show --candidate-chunks:\n%s", help.String())
	}
	if strings.Contains(help.String(), "--top-k") || strings.Contains(help.String(), "deprecated") {
		t.Errorf("help must not mention removed --top-k:\n%s", help.String())
	}
}
