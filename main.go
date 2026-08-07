/*
Copyright © 2026 nmdra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/x/term"

	"github.com/nmdra/notebrain-cli/v2/cmd"
)

//go:embed config.example.toml
var defaultConfigFile []byte

// Populated automatically by GoReleaser during git tag builds:
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Exit codes:
//
//	0 - success
//	1 - runtime or operational failure
//	2 - command-line usage error
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(runMain(os.Args[1:]))
}

func runMain(args []string) (code int) {
	defer recoverMain(&code)

	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}

	ctx := context.Background()
	err := cmd.ParseAndRun(ctx, version, commit, date, defaultConfigFile, args)
	if err == nil {
		return exitOK
	}

	if _, ok := errors.AsType[*cmd.UsageError](err); ok {
		return exitUsage
	}

	// The error was already emitted as {"error": ...} on stdout; the stderr
	// line would be redundant.
	if _, ok := errors.AsType[*cmd.JSONEnvelopeError](err); ok {
		return exitError
	}

	printFatalError(err)
	return exitError
}

// recoverMain handles a panic by logging the stack and converting it into a
// failure exit code. It must be called from a deferred function so that
// recover() is active.
func recoverMain(code *int) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		slog.Error("internal error: panic recovered", "panic", fmt.Sprint(r), "stack", string(stack))
		fmt.Fprintf(os.Stderr, "internal error: %v\n", r)
		*code = exitError
	}
}

// printFatalError writes the final error line to stderr, colored red when
// stderr is an interactive terminal and colors are allowed.
func printFatalError(err error) {
	msg := fmt.Sprintf("Error: %v\n", err)
	if term.IsTerminal(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb" && os.Getenv("NO_COLOR") == "" {
		msg = "\x1b[1;31m" + msg + "\x1b[0m"
	}
	fmt.Fprintf(os.Stderr, "%s", msg)
}
