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
	"github.com/nmdra/notebrain-cli/internal/configfile"
)

// Globals holds shared configuration available to all subcommands.
type Globals struct {
	ChromaPath     string  `help:"path to ChromaDB persistent storage" default:"~/.notebrain/chroma"`
	VaultPath      string  `name:"vault-path" help:"Obsidian vault path (also used as vault name fallback)"`
	VaultName      string  `name:"vault-name" help:"Obsidian vault name (for URI links, defaults to basename of vault-path)"`
	Verbose        bool    `help:"enable verbose output"`
	NoHyperlinks   bool    `help:"Disable OSC 8 terminal hyperlinks in output"`
	Format         string  `help:"output format" enum:"text,json,tsv,ndjson" default:"text"`
	JSONPath       string  `name:"jsonpath" help:"extract specific fields from JSON output using a JSONPath expression (e.g. '$.results[0].note_slug')"`
	IncludeText    bool    `help:"include matched chunk text in structured output"`
	ContextWindow  int     `name:"context-window" help:"fetch ±N adjacent chunks around each match for context" default:"0"`
	MinScore       float64 `help:"suppress results below this similarity score (0–1)" default:"0"`
	RespectExclude bool    `help:"respect Obsidian userIgnoreFilters and attachmentFolderPath settings during ingest" default:"true"`
	UseEditor      bool    `help:"enable external editor ($EDITOR) integration as default open type" default:"false"`
	LogFormat      string  `name:"log-format" help:"log output format (auto, json, text)" default:"auto"`
	LogLevel       string  `name:"log-level" help:"minimum log severity (info, debug, warn, error)" default:"info"`

	// Internal fields, not exposed as flags
	Ctx context.Context `kong:"-"`

	Config kong.ConfigFlag `help:"Path to config file" default:"~/.notebrain/config/config.toml"`
}

// CLI is the top-level Kong command tree.
type CLI struct {
	Globals

	Ingest      IngestCmd      `cmd:"" help:"Ingest markdown files from a vault"`
	Search      SearchCmd      `cmd:"" help:"Semantic search across indexed notes"`
	Backlinks   BacklinksCmd   `cmd:"" help:"Find incoming links to a note"`
	Connections ConnectionsCmd `cmd:"" help:"Traverse graph connections"`
	Hidden      HiddenCmd      `cmd:"" help:"Discover hidden semantic links between unlinked notes"`
	Tags        TagsCmd        `cmd:"" help:"Find notes sharing common tags"`
	Boosted     BoostedCmd     `cmd:"" help:"Graph-boosted semantic search"`
	Stats       StatsCmd       `cmd:"" help:"Show collection statistics"`
	Get         GetCmd         `cmd:"" help:"Fetch complete note content by slug or path"`
	Reset       ResetCmd       `cmd:"" help:"Reset the ChromaDB collections"`
}

// ParseAndRun parses CLI arguments and runs the selected subcommand.
func ParseAndRun(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	defaultChromaPath := filepath.Join(home, ".notebrain", "chroma")

	cli := CLI{
		Globals: Globals{
			ChromaPath: defaultChromaPath,
			Ctx:        ctx,
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
		if cli.Format == "json" || cli.Format == "ndjson" {
			// Print error as JSON to stdout for agents
			_, _ = fmt.Fprintf(os.Stdout, "{\"error\": %q}\n", err.Error())
			os.Exit(1)
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
