package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestRunMainExitCodes(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no user config, no user chroma path
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"usage error", []string{"--bogus-flag"}, exitUsage},
		{"missing query", []string{"search"}, exitUsage}, // missing query is a usage error
		{"missing vault", []string{"ingest"}, exitUsage}, // missing --vault-path is a usage error
		{"bad log level", []string{"search", "q", "--log-level=verbose"}, exitUsage},
		{"version subcommand", []string{"version"}, exitOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runMain(tt.args); got != tt.want {
				t.Errorf("runMain(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}

func TestPanicRecovery(t *testing.T) {
	code := 0
	func() {
		defer recoverMain(&code)
		panic("boom")
	}()
	if code != exitError {
		t.Errorf("panic recovery exit code = %d, want %d", code, exitError)
	}
}

// captureRunMain runs runMain while capturing stdout and stderr.
func captureRunMain(t *testing.T, args []string) (code int, stdout, stderr string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	or, ow, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	er, ew, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = ow, ew

	code = runMain(args)

	os.Stdout, os.Stderr = oldOut, oldErr
	_ = ow.Close()
	_ = ew.Close()
	out, _ := io.ReadAll(or)
	errd, _ := io.ReadAll(er)
	return code, string(out), string(errd)
}

// TestRunMainJSONModeSingleErrorLine verifies that --format json failures
// emit exactly one error representation: the {"error": ...} envelope on
// stdout, with no duplicate "Error:" line on stderr. A temp chroma path is
// used so the run fails fast on an empty database (the chroma-go shim cache
// in the real HOME is still resolved).
func TestRunMainJSONModeSingleErrorLine(t *testing.T) {
	args := []string{"get", "nonexistent-note", "--chroma-path", t.TempDir(), "--format", "json"}

	code, stdout, stderr := captureRunMain(t, args)
	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, `{"error":`) {
		t.Errorf("expected JSON error envelope on stdout, got: %s", stdout)
	}
	if strings.Contains(stderr, "Error:") {
		t.Errorf("JSON mode must not duplicate the error on stderr, got: %s", stderr)
	}
}

func TestRunMainTextModeStderrError(t *testing.T) {
	args := []string{"get", "nonexistent-note", "--chroma-path", t.TempDir()}

	_, _, stderr := captureRunMain(t, args)
	if !strings.Contains(stderr, "Error:") {
		t.Errorf("text mode must print the error on stderr, got: %s", stderr)
	}
}
