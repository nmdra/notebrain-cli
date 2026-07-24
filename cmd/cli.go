package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/x/term"

	"github.com/nmdra/notebrain-cli/v2/internal/configfile"
)

// ChunkDisplayFlags holds options for semantic search commands that return text chunks.
type ChunkDisplayFlags struct {
	IncludeText   bool    `help:"include the matched markdown text in each result"`
	ContextWindow int     `name:"context-window" help:"fetch ±N surrounding chunks around each match (0=off)" default:"0"`
	MinScore      float64 `help:"minimum similarity score to include in results (0.0–1.0)" default:"0"`
}

// Globals holds shared configuration available to all subcommands.
type Globals struct {
	ChromaPath      string           `help:"path to ChromaDB persistent storage directory" default:"~/.notebrain/chroma"`
	VaultPath       string           `name:"vault-path" help:"path to the Obsidian vault directory"`
	VaultName       string           `name:"vault-name" help:"vault display name for Obsidian URI links (defaults to basename of --vault-path)"`
	Verbose         bool             `help:"show detailed output including all matched sections"`
	NoHyperlinks    bool             `help:"disable clickable terminal hyperlinks in output"`
	Format          string           `help:"output format: text, json, or tsv" enum:"text,json,tsv" default:"text"`
	JSONPath        string           `name:"jsonpath" help:"extract fields using JSONPath (e.g. '$.results[*].note_slug')"`
	LogFormat       string           `name:"log-format" help:"log output format (auto, json, text)" default:"auto"`
	LogLevel        string           `name:"log-level" help:"minimum log severity (info, debug, warn, error)" default:"info"`
	SkipAttachments bool             `name:"skip-attachments" help:"exclude image/attachment links from graph edges" default:"true"`
	SkipPhantom     bool             `name:"skip-phantom" help:"exclude phantom (uncreated) notes from results" default:"true"`
	HideTags        bool             `name:"hide-tags" help:"hide tag names from output (use --hide-tags=false to show)" default:"true"`
	ShowFilePath    bool             `name:"show-file-path" help:"include file_path in output (use --show-file-path=false to hide)" default:"true"`
	Version         kong.VersionFlag `name:"version" help:"show version information"`

	// Internal fields, not exposed as flags
	Ctx           context.Context `kong:"-"`
	VersionString string          `kong:"-"`
	Queries       []string        `kong:"-"`

	Config kong.ConfigFlag `help:"path to config file" default:"~/.notebrain/config/config.toml"`
}

// CLI is the top-level Kong command tree.
type CLI struct {
	Globals

	Ingest      IngestCmd      `cmd:"" help:"Ingest markdown files from a vault"`
	Search      SearchCmd      `cmd:"" help:"Semantic search across indexed notes"`
	Backlinks   BacklinksCmd   `cmd:"" help:"Find incoming links to a note"`
	Connections ConnectionsCmd `cmd:"" help:"Find notes connected via wikilinks (graph traversal)"`
	Hidden      HiddenCmd      `cmd:"" help:"Discover semantically related but unlinked notes"`
	Tags        TagsCmd        `cmd:"" help:"Find notes sharing common tags"`
	Boosted     BoostedCmd     `cmd:"" help:"Semantic search boosted by wikilink graph proximity"`
	Stats       StatsCmd       `cmd:"" help:"Show collection statistics"`
	Get         GetCmd         `cmd:"" help:"Retrieve the full text of an indexed note"`
	Reset       ResetCmd       `cmd:"" help:"Delete all indexed data and reset the database"`
	Version     VersionCmd     `cmd:"" help:"Show version information"`
}

// ParseAndRun parses CLI arguments and runs the selected subcommand.
func ParseAndRun(ctx context.Context, version, commit, date string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	defaultChromaPath := filepath.Join(home, ".notebrain", "chroma")

	versionStr := fmt.Sprintf("notebrain %s (commit: %s, built: %s)", version, commit, date)

	cli := CLI{
		Globals: Globals{
			ChromaPath:    defaultChromaPath,
			Ctx:           ctx,
			VersionString: versionStr,
		},
	}

	ctxParser := kong.Parse(&cli,
		kong.Name("notebrain"),
		kong.Description(`Index and search your Obsidian vault with semantic intelligence.

NoteBrain uses local LLM embeddings to index your Markdown notes into ChromaDB,
enabling powerful semantic search, hidden graph connections, and AI-friendly automation workflows.

Examples:
  # Ingest your entire vault into ChromaDB
  notebrain ingest --vault-path "/path/to/Obsidian"

  # Perform a semantic search across your notes
  notebrain search "how to configure neovim" --limit 5

  # Graph-boosted search (combines semantic similarity + wikilink connections)
  notebrain boosted "docker setup"

  # Find hidden connections between notes that are not explicitly linked
  notebrain hidden "project alpha"

  # Automate CLI output for AI agents (Claude, Gemini, etc.)
  notebrain search "rust error handling" --format json --include-text`),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: false,
		}),
		kong.Configuration(configfile.IgnoreMissingFileLoader(configfile.TOMLResolver)),
		kong.Vars{"version": versionStr},
	)

	setupLogger(cli.LogFormat, cli.LogLevel)

	// Resolve vault display name for Obsidian URI generation.
	// Priority: --vault-name flag / config > basename(vault-path)
	vaultName := cli.VaultName
	if vaultName == "" && cli.VaultPath != "" {
		vaultName = filepath.Base(cli.VaultPath)
	}
	cli.VaultName = vaultName

	if strings.HasPrefix(cli.ChromaPath, "~/") {
		if home != "" && home != "." {
			cli.ChromaPath = filepath.Join(home, cli.ChromaPath[2:])
		}
	}

	err = ctxParser.Run(&cli.Globals)
	if err != nil {
		if cli.Format == formatJSON || cli.Format == formatNDJSON {
			// Print error as JSON to stdout for agents
			_, _ = fmt.Fprintf(os.Stdout, "{\"error\": %q}\n", err.Error())
		}
		return err
	}
	return nil
}

func setupLogger(logFormat, logLevel string) {
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default: // "info"
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler

	isTTY := term.IsTerminal(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb"
	format := strings.ToLower(logFormat)

	if format == "json" || (format == "auto" && !isTTY) {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// hyperlinkSupported returns true if the terminal supports OSC 8 hyperlinks
// and the user has not disabled them.
func hyperlinkSupported(globals *Globals) bool {
	if globals.NoHyperlinks || os.Getenv("NO_HYPERLINKS") != "" {
		return false
	}
	return isHyperlinkSupportedEnv()
}

func isHyperlinkSupportedEnv() bool {
	term := os.Getenv("TERM")
	prog := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	color := strings.ToLower(os.Getenv("COLORTERM"))

	switch prog {
	case "iterm.app", "wezterm", "ghostty", "hyper":
		return true
	}
	if color == "truecolor" || color == "24bit" {
		return true
	}
	if strings.HasPrefix(term, "xterm-kitty") || strings.HasPrefix(term, "foot") {
		return true
	}
	if os.Getenv("WT_SESSION") != "" {
		return true
	}
	return false
}
