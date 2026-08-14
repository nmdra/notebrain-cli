package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// searchParser builds a kong parser bound to a SearchCmd so flag-level
// behavior (alias parsing, help visibility) can be asserted without running
// the command.
func searchParser(t *testing.T) (*kong.Kong, *SearchCmd) {
	t.Helper()
	var cli struct {
		Globals Globals   `embed:""`
		Search  SearchCmd `cmd:""`
	}
	parser, err := kong.New(&cli, kong.ExplicitGroups(helpGroups()))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	return parser, &cli.Search
}

func TestSearchExcludeNotesFlagRename(t *testing.T) {
	parser, search := searchParser(t)

	t.Run("exclude-notes accepts one value", func(t *testing.T) {
		if _, err := parser.Parse([]string{"search", "q", "--exclude-notes", "alpha.md"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(search.ExcludeNotes) != 1 || search.ExcludeNotes[0] != "alpha.md" {
			t.Errorf("ExcludeNotes = %v, want [alpha.md]", search.ExcludeNotes)
		}
	})

	t.Run("exclude-notes accepts repeats", func(t *testing.T) {
		if _, err := parser.Parse([]string{"search", "q", "--exclude-notes", "alpha.md", "--exclude-notes", "beta.md"}); err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(search.ExcludeNotes) != 2 {
			t.Errorf("ExcludeNotes = %v, want 2 entries", search.ExcludeNotes)
		}
	})

	t.Run("legacy exclude-note alias rejected", func(t *testing.T) {
		if _, err := parser.Parse([]string{"search", "q", "--exclude-note", "alpha.md"}); err == nil {
			t.Error("parse legacy alias: expected error, got none")
		}
	})

	var help strings.Builder
	parser.Stdout = &help
	ctx, err := parser.Parse([]string{"search", "q"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatalf("PrintUsage: %v", err)
	}
	if !strings.Contains(help.String(), "--exclude-notes") {
		t.Errorf("help must show --exclude-notes:\n%s", help.String())
	}
	if strings.Contains(strings.ReplaceAll(help.String(), "--exclude-notes", ""), "--exclude-note") {
		t.Errorf("help must not mention --exclude-note:\n%s", help.String())
	}
}

func TestSearchExcludeNotesMultipleFlags(t *testing.T) {
	var cli struct {
		Globals Globals   `embed:""`
		Search  SearchCmd `cmd:""`
	}
	parser, err := kong.New(&cli, kong.ExplicitGroups(helpGroups()))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := parser.Parse([]string{"search", "q", "--exclude-notes", "a.md", "--exclude-notes", "b.md"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cli.Search.ExcludeNotes) != 2 {
		t.Errorf("excludes = %v, want [a.md b.md]", cli.Search.ExcludeNotes)
	}
}
