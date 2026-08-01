package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"
	"github.com/posener/complete"
)

// TestKongCompletionRegisterCompat verifies that kong-completion v0.0.14
// registers against our kong v1.16.0 model without panicking, exiting,
// or intercepting normal (non-completion) invocations.
func TestKongCompletionRegisterCompat(t *testing.T) {
	cli := CLI{}
	parser := kong.Must(&cli,
		kong.Name("notebrain"),
		kong.Exit(func(int) {}),
	)
	kongcompletion.Register(parser)
}

// TestCompletionCommandOutputsActivation verifies the `completion` subcommand
// prints per-shell activation instructions.
func TestCompletionCommandOutputsActivation(t *testing.T) {
	cases := map[string]string{
		"bash": "source <(notebrain completion -c bash)",
		"zsh":  "source <(notebrain completion -c zsh)",
		"fish": "notebrain completion -c fish | source",
	}
	for shell, want := range cases {
		t.Run(shell, func(t *testing.T) {
			cli := CLI{}
			var buf bytes.Buffer
			parser := kong.Must(&cli,
				kong.Name("notebrain"),
				kong.Exit(func(int) {}),
			)
			parser.Stdout = &buf
			kongcompletion.Register(parser, completionPredictors()...)

			ctx, err := parser.Parse([]string{"completion", shell})
			if err != nil {
				t.Fatalf("Parse(%q) failed: %v", shell, err)
			}
			if err := ctx.Run(); err != nil {
				t.Fatalf("Run(%q) failed: %v", shell, err)
			}
			if !strings.Contains(buf.String(), want) {
				t.Errorf("activation output for %q does not contain %q:\n%s", shell, want, buf.String())
			}
		})
	}
}

// TestCompletionInterceptsShellRequests simulates a bash `complete -C`
// invocation (COMP_LINE set) and asserts the predicted options for enum
// flags and subcommands.
func TestCompletionInterceptsShellRequests(t *testing.T) {
	t.Run("enum flag values", func(t *testing.T) {
		t.Setenv("COMP_LINE", "notebrain --format ")

		var buf bytes.Buffer
		cli := CLI{}
		parser := kong.Must(&cli,
			kong.Name("notebrain"),
			kong.Exit(func(int) {}),
		)
		parser.Stdout = &buf
		kongcompletion.Register(parser, completionPredictors()...)

		got := buf.String()
		for _, want := range []string{"text", "json", "tsv"} {
			if !strings.Contains(got, want) {
				t.Errorf("completion output missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "ingest") {
			t.Errorf("subcommands must not leak while completing a flag value:\n%s", got)
		}
	})

	t.Run("subcommands", func(t *testing.T) {
		t.Setenv("COMP_LINE", "notebrain ")

		var buf bytes.Buffer
		cli := CLI{}
		parser := kong.Must(&cli,
			kong.Name("notebrain"),
			kong.Exit(func(int) {}),
		)
		parser.Stdout = &buf
		kongcompletion.Register(parser, completionPredictors()...)

		got := buf.String()
		for _, want := range []string{"ingest", "search", "get", "completion", "stats", "doctor"} {
			if !strings.Contains(got, want) {
				t.Errorf("completion output missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "suggest-notes") {
			t.Errorf("hidden command suggest-notes must not be completed:\n%s", got)
		}
	})
}

// TestCompletionPredictorTags verifies registered predictor names are
// wired to the intended flags.
func TestCompletionPredictorTags(t *testing.T) {
	cli := CLI{}
	parser := kong.Must(&cli, kong.Name("notebrain"))
	kongcompletion.Register(parser, completionPredictors()...)

	cmd, err := kongcompletion.Command(parser, completionPredictors()...)
	if err != nil {
		t.Fatalf("Command() failed: %v", err)
	}
	if _, ok := cmd.Sub["ingest"].GlobalFlags["--llm-model"]; !ok {
		t.Error("--llm-model not completed for the ingest command")
	}
	if _, ok := cmd.GlobalFlags["--format"]; !ok {
		t.Error("--format (global enum flag) not completed at the root")
	}
	if _, ok := cmd.GlobalFlags["--log-level"]; !ok {
		t.Error("--log-level (global enum flag) not completed at the root")
	}
}

// TestNoteSlugPredictorWiring verifies the note-slug predictor is attached
// to positional note args and --seed, and that the predictor is invoked
// (spawning suggest-notes) rather than a static set.
func TestNoteSlugPredictorWiring(t *testing.T) {
	cli := CLI{}
	parser := kong.Must(&cli, kong.Name("notebrain"))
	kongcompletion.Register(parser, completionPredictors()...)

	cmd, err := kongcompletion.Command(parser, completionPredictors()...)
	if err != nil {
		t.Fatalf("Command() failed: %v", err)
	}

	for _, sub := range []string{"get", "backlinks", "connections", "hidden", "tags"} {
		args, ok := cmd.Sub[sub].Args.(*kongcompletion.PositionalPredictor)
		if !ok {
			t.Fatalf("%s: expected PositionalPredictor, got %T", sub, cmd.Sub[sub].Args)
		}
		if len(args.Predictors) == 0 {
			t.Fatalf("%s: no positional predictor attached", sub)
		}
		if _, ok := args.Predictors[0].(complete.PredictFunc); !ok {
			t.Errorf("%s: positional predictor is %T, want complete.PredictFunc", sub, args.Predictors[0])
		}
	}

	seed, ok := cmd.Sub["boosted"].GlobalFlags["--seed"]
	if !ok {
		t.Fatal("--seed not completed for the boosted command")
	}
	if _, ok := seed.(complete.PredictFunc); !ok {
		t.Errorf("--seed predictor is %T, want complete.PredictFunc", seed)
	}
}

// TestStripCompletionEnv verifies COMP_LINE/COMP_POINT are removed from the
// environment of the suggest-notes child process (posener/complete hijacks
// on COMP_LINE alone), while other variables are preserved.
func TestStripCompletionEnv(t *testing.T) {
	t.Setenv("COMP_LINE", "notebrain get ")
	t.Setenv("COMP_POINT", "15")
	t.Setenv("KEEP_ME", "yes")

	env := stripCompletionEnv()
	var kept, sawLine, sawPoint bool
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "COMP_LINE="):
			sawLine = true
		case strings.HasPrefix(kv, "COMP_POINT="):
			sawPoint = true
		case kv == "KEEP_ME=yes":
			kept = true
		}
	}
	if sawLine {
		t.Error("COMP_LINE leaked into suggest-notes environment")
	}
	if sawPoint {
		t.Error("COMP_POINT leaked into suggest-notes environment")
	}
	if !kept {
		t.Error("unrelated environment variables were dropped")
	}
}
