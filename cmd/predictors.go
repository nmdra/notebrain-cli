package cmd

import (
	"os"
	"os/exec"
	"strings"

	"github.com/posener/complete"
)

// predictNoteSlugs returns all indexed note slugs for dynamic completion.
// It shells out to the hidden suggest-notes command; a failure (e.g. no
// database yet) yields no candidates rather than an error, so completion
// never crashes or hangs.
func predictNoteSlugs(_ complete.Args) []string {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	cmd := exec.Command(exe, "suggest-notes")
	cmd.Env = stripCompletionEnv()
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// stripCompletionEnv removes COMP_LINE/COMP_POINT from the environment so
// the spawned suggest-notes process is not hijacked by completion
// interception (posener/complete triggers on COMP_LINE alone).
func stripCompletionEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "COMP_LINE=") || strings.HasPrefix(kv, "COMP_POINT=") {
			continue
		}
		env = append(env, kv)
	}
	return env
}
