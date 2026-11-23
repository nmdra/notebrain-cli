package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

func TestMCPConfigSnapshotUsesActualResolverPrecedence(t *testing.T) {
	var resolvers []kong.Resolver
	loader := captureMCPConfig(&resolvers)
	first, err := loader(strings.NewReader("limit=7\ninclude_text=true\nexclude_notes=['old']\n"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := loader(strings.NewReader("limit=9\ninclude-text=false\ninclude_text=true\nexclude_notes=[]\n"))
	if err != nil {
		t.Fatal(err)
	}
	cli := CLI{}
	parser, err := kong.New(&cli, kong.Vars{"version": "test"}, kong.Resolvers(first, second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"search", "example"}); err != nil {
		t.Fatal(err)
	}
	values, err := mcpConfigSnapshot(resolvers, parser)
	if err != nil {
		t.Fatal(err)
	}
	if values["limit"] != strconv.Itoa(cli.Search.Limit) {
		t.Errorf("limit snapshot=%v CLI=%v", values["limit"], cli.Search.Limit)
	}
	if values["include-text"] != "false" || cli.Search.IncludeText {
		t.Errorf("exact key precedence lost: %v / %v", values["include-text"], cli.Search.IncludeText)
	}
	if _, ok := values["hops"]; ok {
		t.Error("absent command default must stay absent")
	}
	if values["exclude-notes"] != "[]" || len(cli.Search.ExcludeNotes) != 0 {
		t.Fatalf("explicit config array = %#v", cli.Search.ExcludeNotes)
	}
}

func TestMCPChildDefaultConfigIsolation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("COMP_LINE", "notebrain search some")
	t.Setenv("COMP_POINT", "26")
	logPath := filepath.Join(home, "do-not-create.log")
	t.Setenv("NOTEBRAIN_LOG_FILE", logPath)
	dir := filepath.Join(home, ".notebrain", "config")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("broken = ["), 0600); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(t.TempDir(), "profile.toml")
	if err := os.WriteFile(private, []byte("show_tags=true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	chroma := t.TempDir()
	if err := os.WriteFile(filepath.Join(chroma, "chroma.sqlite3"), []byte("SQLite format 3\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	withFakeStore(t, &fakeStore{stats: &store.Stats{}})
	out := captureStdout(t, func() {
		err := ParseAndRun(context.Background(), "test", "none", "now", nil, []string{"--mcp-child", "--config=" + private, "--chroma-path=" + chroma, "--format=tsv", "--jsonpath=$.notes", "stats"})
		if err != nil {
			t.Errorf("private child failed: %v", err)
		}
	})
	if !strings.Contains(out, `"command": "stats"`) {
		t.Errorf("child output = %q", out)
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("child created configured log file: %v", err)
	}
}

func TestMCPChildRequiresPrivateConfigAndAllowlist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	private := filepath.Join(t.TempDir(), "profile.toml")
	if err := os.WriteFile(private, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--mcp-child", "stats"},
		{"--mcp-child=false", "--config=" + private, "stats"},
		{"--mcp-child", "--config=" + private, "reset"},
	} {
		out := captureStdout(t, func() {
			if err := ParseAndRun(context.Background(), "test", "none", "now", nil, args); err == nil {
				t.Errorf("accepted %v", args)
			}
		})
		if out != "" {
			t.Errorf("stdout contaminated: %q", out)
		}
	}
	if err := os.Chmod(private, 0644); err != nil {
		t.Fatal(err)
	}
	withFakeStore(t, &fakeStore{stats: &store.Stats{}})
	captureStdout(t, func() {
		err := ParseAndRun(context.Background(), "test", "none", "now", nil, []string{"--mcp-child", "--config=" + private, "stats"})
		if err == nil || !strings.Contains(err.Error(), "private") {
			t.Errorf("non-private config accepted: %v", err)
		}
	})
}
