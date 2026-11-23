package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMCPRunnerHelper(t *testing.T) {
	if os.Getenv("NB_BRIDGE_HELPER") != "1" {
		return
	}
	args := os.Args
	sep := 0
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	args = args[sep+1:]
	mode := os.Getenv("NB_BRIDGE_MODE")
	switch mode {
	case "overflow":
		fmt.Print(strings.Repeat("x", 4*1024*1024+1))
	case "stderr":
		fmt.Fprint(os.Stderr, strings.Repeat("x", 128*1024))
		fmt.Print(`{}`)
	case "wait":
		time.Sleep(time.Minute)
	case "fail":
		fmt.Fprint(os.Stderr, "secret note body")
		os.Exit(4)
	default:
		var profile string
		for _, a := range args {
			if a == "--" {
				break
			}
			if after, ok := strings.CutPrefix(a, "--config="); ok {
				profile = after
			}
		}
		data, e := os.ReadFile(profile)
		if e != nil {
			os.Exit(5)
		}
		info, _ := os.Stat(profile)
		dir, _ := os.Stat(strings.TrimSuffix(profile, "/"+lastPart(profile)))
		stdin, _ := io.ReadAll(os.Stdin)
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"args": args, "profile": string(data), "mode": info.Mode().Perm(), "dirmode": dir.Mode().Perm(), "stdin": string(stdin), "comp": os.Getenv("COMP_LINE"), "key": os.Getenv("OPENROUTER_API_KEY")})
	}
	os.Exit(0)
}
func lastPart(p string) string { i := strings.LastIndex(p, "/"); return p[i+1:] }
func newHelperRunner(t *testing.T, mode string, defaults map[string]string) *Runner {
	t.Helper()
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("NB_BRIDGE_HELPER", "1")
	t.Setenv("NB_BRIDGE_MODE", mode)
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	r, e := NewRunner(exe, []string{"-test.run=^TestMCPRunnerHelper$", "--"}, defaults)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { r.Close() })
	return r
}
func TestMCPRunner(t *testing.T) {
	t.Setenv("COMP_LINE", "secret")
	t.Setenv("OPENROUTER_API_KEY", "secret")
	r := newHelperRunner(t, "", map[string]string{"limit": "10", "exclude-notes": `["old"]`})
	out, e := r.Execute(context.Background(), Call{Command: "search", Args: []string{"--config=/evil", "a b 世界"}, Overrides: map[string]string{"exclude-notes": "[]", "limit": "0"}})
	if e != nil {
		t.Fatal(e)
	}
	var got struct {
		Args             []string
		Profile          string
		Mode, DirMode    uint32
		Stdin, Comp, Key string
	}
	if e = json.Unmarshal(out, &got); e != nil {
		t.Fatal(e)
	}
	if got.Mode != 0600 || got.DirMode != 0700 || got.Stdin != "" || got.Comp != "" || got.Key != "" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Profile, "exclude-notes = []") || strings.Contains(got.Profile, "limit") {
		t.Fatal(got.Profile)
	}
	joined := strings.Join(got.Args, "|")
	if !strings.Contains(joined, "--mcp-child") || !strings.Contains(joined, "--format=json") || !strings.Contains(joined, "--jsonpath=") || !strings.Contains(joined, "--|--config=/evil|a b 世界") {
		t.Fatal(joined)
	}
}
func TestMCPRunnerFailures(t *testing.T) {
	for _, mode := range []string{"overflow", "fail", "wait", "stderr"} {
		t.Run(mode, func(t *testing.T) {
			r := newHelperRunner(t, mode, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			data, e := r.Execute(ctx, Call{Command: "stats"})
			if mode == "stderr" {
				if e != nil || string(data) != "{}" {
					t.Fatalf("%s %v", data, e)
				}
			} else if e == nil || strings.Contains(e.Error(), "secret") {
				t.Fatalf("error=%v", e)
			}
		})
	}
}
