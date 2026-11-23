package main

import "testing"

func TestMCPMainStartupError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, flag := range []string{"--bad-option", "--max-concurrent-requests=0", "--max-concurrent-requests=17", "--log-level=verbose"} {
		code, out, errout := captureRunMain(t, []string{"mcp", "serve", "--format=json", flag})
		if code != exitUsage {
			t.Errorf("%s: code = %d, want %d", flag, code, exitUsage)
		}
		if out != "" {
			t.Errorf("%s: protocol stdout contaminated: %q", flag, out)
		}
		if errout == "" {
			t.Errorf("%s: missing stderr diagnostic", flag)
		}
	}
}
