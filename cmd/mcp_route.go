package cmd

import (
	"errors"
	"fmt"

	"github.com/alecthomas/kong"
)

type mcpRoute struct {
	server         bool
	child          bool
	explicitConfig bool
	bare           bool
}

func (r mcpRoute) isolated() bool { return r.server || r.child }

// Trace command nodes, not argv substrings, before config and completion hooks.
func traceMCPRoute(version string, args []string) (mcpRoute, error) {
	route := mcpRoute{}
	grammar, err := kong.New(&CLI{}, kong.Vars{cliVersionKey: version})
	if err != nil {
		return route, fmt.Errorf("construct CLI grammar: %w", err)
	}
	trace, err := kong.Trace(grammar, args)
	if err != nil {
		return route, fmt.Errorf("trace CLI grammar: %w", err)
	}
	for _, path := range trace.Path {
		if path.Command != nil && path.Command.Name == "mcp" {
			route.server = true
		}
		if path.Flag != nil {
			switch path.Flag.Name {
			case "mcp-child":
				route.child = true
			case "config":
				route.explicitConfig = true
			}
		}
	}
	route.bare = route.server && len(args) == 1
	return route, nil
}

func (r mcpRoute) parseFailure(parser *kong.Kong, err error) error {
	// A bare `mcp` is a command group, not a server invocation.
	var parseErr *kong.ParseError
	if r.bare && errors.As(err, &parseErr) && parseErr.Context != nil {
		if helpErr := parseErr.Context.PrintUsage(false); helpErr != nil {
			return fmt.Errorf("print MCP command help: %w", helpErr)
		}
		return nil
	}
	parser.Errorf("%s", err)
	return &UsageError{Err: err}
}
