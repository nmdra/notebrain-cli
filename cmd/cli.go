package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/nmdra/notebrain-cli/internal/configfile"
)

// Globals holds shared configuration available to all subcommands.
type Globals struct {
	ChromaPath   string  `help:"path to ChromaDB persistent storage" default:"~/.notebrain/chroma"`
	ChromaMode   string  `help:"ChromaDB client mode ('persistent' or 'http')" default:"persistent"`
	ChromaURL    string  `help:"ChromaDB server URL (used when --chroma-mode=http)" default:"http://localhost:8000"`
	VaultPath    string  `help:"Obsidian vault path (also used as vault name fallback)"`
	Verbose      bool    `help:"enable verbose output"`
	NoHyperlinks bool    `help:"Disable OSC 8 terminal hyperlinks in output"`
	Format       string  `help:"output format" enum:"text,json,tsv,ndjson" default:"text"`
	IncludeText  bool    `help:"include matched chunk text in structured output"`
	MinScore     float64 `help:"suppress results below this similarity score (0–1)" default:"0"`

	// Internal fields, not exposed as flags
	Ctx       context.Context `kong:"-"`
	VaultName string          `kong:"-"` // resolved display name for Obsidian URIs

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
		kong.Description("Index and search your Obsidian vault with semantic intelligence"),
		kong.UsageOnError(),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: true,
		}),
		kong.Configuration(configfile.IgnoreMissingFileLoader(configfile.TOMLResolver)),
	)

	// Resolve vault display name for Obsidian URI generation.
	// Priority: --vault-path flag > OBSIDIAN_VAULT_NAME > OBSIDIAN_VAULT > basename(OBSIDIAN_VAULT_PATH)
	vault := cli.VaultPath
	if vault == "" {
		vault = os.Getenv("OBSIDIAN_VAULT_NAME")
		if vault == "" {
			legacyVault := os.Getenv("OBSIDIAN_VAULT")
			if legacyVault != "" {
				vault = legacyVault
			} else {
				vaultPath := os.Getenv("OBSIDIAN_VAULT_PATH")
				if vaultPath != "" {
					vault = vaultPath
				}
			}
		}
	}
	if vault != "" {
		vault = filepath.Base(vault)
	}
	cli.VaultName = vault

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
