package cmd

import (
	"testing"

	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"
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
