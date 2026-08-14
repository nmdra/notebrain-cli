package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

// ingestParser builds a kong parser bound to an IngestCmd so flag-level
// behavior (alias parsing, help visibility) can be asserted without running
// the command.
func ingestParser(t *testing.T) (*kong.Kong, *IngestCmd) {
	t.Helper()
	var cli struct {
		Globals Globals   `embed:""`
		Ingest  IngestCmd `cmd:""`
	}
	parser, err := kong.New(&cli, kong.ExplicitGroups(helpGroups()))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	return parser, &cli.Ingest
}

func TestIngestWithPDFFlag(t *testing.T) {
	parser, ingest := ingestParser(t)

	for _, tt := range []struct {
		name string
		args []string
	}{
		{name: "with-pdf", args: []string{"ingest", "--with-pdf"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := parser.Parse(tt.args)
			if err != nil {
				t.Fatalf("parse %v: %v", tt.args, err)
			}
			if !strings.HasPrefix(ctx.Command(), "ingest") {
				t.Errorf("command = %q, want ingest", ctx.Command())
			}
		})
	}

	t.Run("legacy enable-pdf alias rejected", func(t *testing.T) {
		if _, err := parser.Parse([]string{"ingest", "--enable-pdf"}); err == nil {
			t.Error("parse --enable-pdf: expected error, got none")
		}
	})

	if _, err := parser.Parse([]string{"ingest", "--with-pdf"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !ingest.WithPDF {
		t.Errorf("--with-pdf parse did not set the command field")
	}

	var help strings.Builder
	parser.Stdout = &help
	ctx, err := parser.Parse([]string{"ingest"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatalf("PrintUsage: %v", err)
	}
	if !strings.Contains(help.String(), "--with-pdf") {
		t.Errorf("help must show --with-pdf:\n%s", help.String())
	}
	if strings.Contains(help.String(), "--enable-pdf") || strings.Contains(help.String(), "deprecated") {
		t.Errorf("help must hide deprecated --enable-pdf alias:\n%s", help.String())
	}

	var cli struct {
		Globals Globals   `embed:""`
		Ingest  IngestCmd `cmd:""`
	}
	flagParser, err := kong.New(&cli, kong.ExplicitGroups(helpGroups()))
	if err != nil {
		t.Fatalf("kong.New: %v", err)
	}
	if _, err := flagParser.Parse([]string{"ingest", "--with-pdf"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.Ingest.WithPDF {
		t.Errorf("--with-pdf did not set WithPDF")
	}
}

func TestIngestFlagValuesDefault(t *testing.T) {
	_, ingest := ingestParser(t)
	if ingest.WithPDF {
		t.Errorf("WithPDF must default false")
	}
}
