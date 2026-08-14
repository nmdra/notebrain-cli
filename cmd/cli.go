package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/x/term"
	kongcompletion "github.com/jotaen/kong-completion"
	"github.com/posener/complete"

	"github.com/nmdra/notebrain-cli/v2/internal/configfile"
	"github.com/nmdra/notebrain-cli/v2/internal/logging"
)

// ChunkDisplayFlags holds options for semantic search commands that return text chunks.
type ChunkDisplayFlags struct {
	IncludeText   bool    `group:"display" help:"include the matched markdown text in each result"`
	ContextWindow int     `group:"display" name:"context-window" help:"fetch ±N surrounding chunks around each match (0=off)" default:"0"`
	MinScore      float64 `group:"display" help:"minimum similarity score to include in results (0.0–1.0)" default:"0"`
	GroupByNote   bool    `group:"display" name:"group-by-note" help:"collapse multiple chunk matches from the same note into one row (keeps the best score; adds a 'N matching chunks' extra)" default:"false"`
}

// Flag group keys. The order here determines the order of the titled
// sections in --help output.
const (
	groupGlobal     = "global"
	groupDisplay    = "display"
	groupIngest     = "ingest"
	groupSearch     = "search"
	groupConn       = "connections"
	groupHidden     = "hidden"
	groupBoosted    = "boosted"
	groupTags       = "tags"
	groupReset      = "reset"
	groupGet        = "get"
	groupRefs       = "refs"
	groupCompletion = "completion"
)

// helpGroups returns the titled flag groups shown in --help output. Flags in
// the "global" group are shared by every command; the remaining groups hold
// command-specific flags so users can spot them without scrolling.
func helpGroups() []kong.Group {
	return []kong.Group{
		{Key: groupGlobal, Title: "Global Flags"},
		{Key: groupDisplay, Title: "Output Flags"},
		{Key: groupIngest, Title: "Ingest Flags"},
		{Key: groupSearch, Title: "Search Flags"},
		{Key: groupConn, Title: "Connections Flags"},
		{Key: groupHidden, Title: "Hidden Flags"},
		{Key: groupBoosted, Title: "Boosted Flags"},
		{Key: groupTags, Title: "Tags Flags"},
		{Key: groupReset, Title: "Reset Flags"},
		{Key: groupGet, Title: "Get Flags"},
		{Key: groupRefs, Title: "Refs Flags"},
	}
}

// Globals holds shared configuration available to all subcommands.
type Globals struct {
	ChromaPath    string           `group:"global" help:"path to ChromaDB persistent storage directory" default:"~/.notebrain/chroma"`
	VaultPath     string           `group:"global" name:"vault-path" help:"path to the Obsidian vault directory"`
	VaultName     string           `group:"global" name:"vault-name" help:"vault display name for Obsidian URI links (defaults to basename of --vault-path)"`
	Format        string           `group:"global" help:"output format: text, json, or tsv" enum:"text,json,tsv" default:"text"`
	JSONPath      string           `group:"global" name:"jsonpath" help:"extract fields using JSONPath (e.g. '$.results[*].note_slug')"`
	LogLevel      string           `group:"global" name:"log-level" placeholder:"debug|info|warn|error" help:"logging severity level (default: info; env: NOTEBRAIN_LOG_LEVEL)" default:""`
	LogFile       string           `group:"global" name:"log-file" help:"write logs to this file (JSON) in addition to stderr; rotates on size (env: NOTEBRAIN_LOG_FILE)"`
	LogMaxSizeMB  int              `group:"global" name:"log-max-size-mb" help:"max size of each log file in MiB before rotation (0 = default 10)" default:"10"`
	LogMaxBackups int              `group:"global" name:"log-max-backups" help:"number of rotated log file backups to keep (0 = default 5)" default:"5"`
	SkipPhantom   bool             `group:"global" name:"skip-phantom" help:"exclude phantom (uncreated) notes from results" default:"true"`
	ShowTags      bool             `group:"global" name:"show-tags" help:"include tag names (#tag) in output" default:"false"`
	ShowFilePath  bool             `group:"global" name:"show-file-path" help:"include file_path in output (use --show-file-path=false to hide)" default:"true"`
	Version       kong.VersionFlag `group:"global" name:"version" help:"show version information"`

	// Internal fields, not exposed as flags
	Ctx           context.Context `kong:"-"`
	VersionString string          `kong:"-"`
	DefaultConfig []byte          `kong:"-"`

	Config kong.ConfigFlag `group:"global" placeholder:"path" help:"path to config file (default: ~/.notebrain/config/config.toml)" type:"path"`
}

// CLI is the top-level Kong command tree.
type CLI struct {
	Globals

	Ingest       IngestCmd                 `cmd:"" help:"Ingest markdown files from a vault"`
	Search       SearchCmd                 `cmd:"" help:"Semantic search across indexed notes"`
	Backlinks    BacklinksCmd              `cmd:"" help:"Find incoming links to a note"`
	Connections  ConnectionsCmd            `cmd:"" help:"Find notes connected via wikilinks (graph traversal)"`
	Hidden       HiddenCmd                 `cmd:"" help:"Discover semantically related but unlinked notes"`
	Tags         TagsCmd                   `cmd:"" help:"Find notes sharing common tags"`
	Boosted      BoostedCmd                `cmd:"" help:"Semantic search boosted by wikilink graph proximity"`
	Stats        StatsCmd                  `cmd:"" help:"Show collection statistics"`
	Get          GetCmd                    `cmd:"" help:"Retrieve the full text of an indexed note"`
	Refs         RefsCmd                   `cmd:"" help:"List a note's attachments and external links"`
	Reset        ResetCmd                  `cmd:"" help:"Delete all indexed data and reset the database"`
	Doctor       DoctorCmd                 `cmd:"" help:"Run diagnostics to check system dependencies and configurations"`
	DoctorProbe  DoctorProbeCmd            `cmd:"" hidden:"" help:"internal: verify the database can be opened (used by doctor)"`
	SuggestNotes SuggestNotesCmd           `cmd:"" hidden:"" help:"internal: list indexed note slugs, one per line (used by shell completion)"`
	Init         InitCmd                   `cmd:"" help:"Initialize NoteBrain configuration interactively"`
	Version      VersionCmd                `cmd:"" help:"Show version information"`
	Completion   kongcompletion.Completion `cmd:"" help:"Output shell code for initializing tab completions"`
}

// completionPredictors returns the named predictors used by shell completion.
// They are shared between the real CLI and tests to prevent drift.
func completionPredictors() []kongcompletion.Option {
	return []kongcompletion.Option{
		kongcompletion.WithPredictor("llm-model",
			complete.PredictSet("openrouter/anthropic/claude-3.5-haiku", "deepseek-chat", "gemini-3.5-flash-lite", "ollama/<model>")),
		kongcompletion.WithPredictor("note-slug", complete.PredictFunc(predictNoteSlugs)),
	}
}

// ParseAndRun parses CLI arguments and runs the selected subcommand.
func ParseAndRun(ctx context.Context, version, commit, date string, defaultConfig []byte, args []string) error {
	return parseAndRun(ctx, version, commit, date, defaultConfig, args)
}

// UsageError marks an error as a command-line usage error. runMain maps it to
// exit code 2, so automation can distinguish bad invocations (2) from
// operational failures (1).
type UsageError struct{ Err error }

// vaultPathUsageError is shared by every command that requires an explicit
// vault location; keep the wording in one place.
const vaultPathUsageError = "--vault-path flag or config file setting must be specified — run 'notebrain init' to create a config"

func (e *UsageError) Error() string { return e.Err.Error() }
func (e *UsageError) Unwrap() error { return e.Err }

// JSONEnvelopeError marks an error that has already been emitted as a
// machine-readable {"error": "..."} object on stdout. runMain skips the
// human-readable stderr line for these, so --format json scripts get exactly
// one error representation.
type JSONEnvelopeError struct{ Err error }

func (e *JSONEnvelopeError) Error() string { return e.Err.Error() }
func (e *JSONEnvelopeError) Unwrap() error { return e.Err }

// usageFailure prints the parse error the way kong's FatalIfErrorf would
// (usage to stdout, error to stderr) and returns a UsageError. In JSON mode
// the error is emitted as a machine-readable object instead of usage text.
func usageFailure(parser *kong.Kong, err error, args []string) error {
	var parseErr *kong.ParseError
	if errors.As(err, &parseErr) && !argsWantJSON(args) && parseErr.Context != nil {
		_ = parseErr.Context.PrintUsage(false)
		fmt.Fprintln(parser.Stdout)
	}
	if argsWantJSON(args) {
		_, _ = fmt.Fprintf(parser.Stdout, "{\"error\": %q}\n", err.Error())
		return &JSONEnvelopeError{Err: &UsageError{Err: err}}
	}
	parser.Errorf("%s", err.Error())
	return &UsageError{Err: err}
}

const (
	flagFormat      = "--format"
	levelDebugLabel = "debug"
	levelInfoLabel  = "info"
	levelWarnLabel  = "warn"
	levelErrorLabel = "error"
)

// argsWantJSON reports whether the raw arguments request --format json. Flag
// values are not applied to the CLI struct when parsing fails, so the raw
// args are the only reliable source for this decision on error paths.
func argsWantJSON(args []string) bool {
	for i := range args {
		arg := args[i]
		if arg == flagFormat && i+1 < len(args) {
			if strings.TrimSpace(args[i+1]) == formatJSON {
				return true
			}
			continue
		}
		if v, ok := strings.CutPrefix(arg, flagFormat+"="); ok && strings.TrimSpace(v) == formatJSON {
			return true
		}
	}
	return false
}

// warnIgnoredOutputFlags tells the user (once, on stderr) that a text-only
// command does not honor --format/--jsonpath. stdout stays clean for
// machine consumers.
func warnIgnoredOutputFlags(w io.Writer, format, jsonpath, cmdName string) {
	switch {
	case format != formatText && jsonpath != "":
		fmt.Fprintf(w, "warning: --format and --jsonpath are ignored by '%s' (output is textual only)\n", cmdName)
	case format != formatText:
		fmt.Fprintf(w, "warning: --format is ignored by '%s' (output is textual only)\n", cmdName)
	case jsonpath != "":
		fmt.Fprintf(w, "warning: --jsonpath is ignored by '%s' (output is textual only)\n", cmdName)
	}
}

// textOnlyCommands are the commands whose output is inherently textual and
// never honors --format/--jsonpath.
var textOnlyCommands = map[string]bool{
	groupIngest:     true,
	"reset":         true,
	"doctor":        true,
	"doctor-probe":  true,
	"init":          true,
	"version":       true,
	groupCompletion: true,
	"suggest-notes": true,
}

func isTextOnlyCommand(cmdName string) bool {
	return textOnlyCommands[cmdName]
}

// validateLogLevel rejects log level values that are not one of the supported
// severities. An empty value means "not set" and defers to the env var and
// default.
func validateLogLevel(logLevel string) error {
	if logLevel == "" {
		return nil
	}
	switch strings.ToLower(logLevel) {
	case levelDebugLabel, levelInfoLabel, levelWarnLabel, levelErrorLabel:
		return nil
	}
	return fmt.Errorf("--log-level must be one of debug, info, warn, error (got %q)", logLevel)
}

func parseAndRun(ctx context.Context, version, commit, date string, defaultConfig []byte, args []string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	defaultChromaPath := filepath.Join(home, ".notebrain", "chroma")

	defaultConfigPath := filepath.Join(home, ".notebrain", "config", "config.toml")

	versionStr := fmt.Sprintf("notebrain %s (commit: %s, built: %s)", version, commit, date)

	cli := CLI{
		Globals: Globals{
			ChromaPath:    defaultChromaPath,
			Ctx:           ctx,
			VersionString: versionStr,
			DefaultConfig: defaultConfig,
		},
	}

	parser := kong.Must(&cli,
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
  notebrain boosted "docker setup" --seed "project alpha"

  # Find hidden connections between notes that are not explicitly linked
  notebrain hidden "project alpha"

  # Automate CLI output for AI agents (Claude, Gemini, etc.)
  notebrain search "rust error handling" --format json --include-text`),
		kong.UsageOnError(),
		kong.ExplicitGroups(helpGroups()),
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			Summary: false,
		}),
		kong.Configuration(configfile.IgnoreMissingFileLoader(configfile.TOMLResolver), defaultConfigPath),
		kong.Vars{"version": versionStr},
	)

	// Intercept shell completion requests (COMP_LINE env) before parsing args.
	kongcompletion.Register(parser, completionPredictors()...)

	ctxParser, err := parser.Parse(args)
	if err != nil {
		return usageFailure(parser, err, args)
	}

	if verr := validateLogLevel(cli.LogLevel); verr != nil {
		return &UsageError{Err: verr}
	}

	logWriter, err := setupLogging(cli.LogLevel, cli.LogFile, cli.LogMaxSizeMB, cli.LogMaxBackups)
	if err != nil {
		return fmt.Errorf("setup logging: %w", err)
	}
	if logWriter != nil {
		defer func() { _ = logWriter.Close() }()
	}

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

	if isTextOnlyCommand(ctxParser.Command()) {
		warnIgnoredOutputFlags(os.Stderr, cli.Format, cli.JSONPath, ctxParser.Command())
	}

	err = ctxParser.Run(&cli.Globals)
	if err != nil {
		if cli.Format == formatJSON {
			// Print error as JSON to stdout for agents; main skips the
			// human-readable stderr line for JSONEnvelopeError.
			_, _ = fmt.Fprintf(os.Stdout, "{\"error\": %q}\n", err.Error())
			return &JSONEnvelopeError{Err: err}
		}
		return err
	}
	if logWriter != nil {
		if werr := logWriter.Err(); werr != nil {
			return fmt.Errorf("log file: %w", werr)
		}
	}
	return nil
}

// resolveLogLevel resolves the effective slog level. Precedence:
// --log-level flag and config file (logLevel) > NOTEBRAIN_LOG_LEVEL
// env var > default info.
func resolveLogLevel(logLevel string) slog.Level {
	level := logLevel
	if level == "" {
		level = os.Getenv("NOTEBRAIN_LOG_LEVEL")
	}
	switch strings.ToLower(level) {
	case levelDebugLabel:
		return slog.LevelDebug
	case levelWarnLabel:
		return slog.LevelWarn
	case levelErrorLabel:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// buildHandler creates a slog handler for the given writer. Text handlers are
// used for interactive terminals; JSON for everything else. Color applies only
// to text handlers.
func buildHandler(level slog.Level, w io.Writer, text, color bool) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if level == slog.LevelDebug {
		opts.AddSource = true
	}
	if text {
		h := slog.NewTextHandler(w, opts)
		if color {
			// The colored handler renders the level as a colored prefix, so
			// drop the plain level attribute from the inner handler.
			noLevel := *opts
			noLevel.ReplaceAttr = func(_ []string, a slog.Attr) slog.Attr {
				if a.Key == slog.LevelKey {
					return slog.Attr{}
				}
				return a
			}
			h = slog.NewTextHandler(w, &noLevel)
			return &coloredTextHandler{inner: h, w: w}
		}
		return h
	}
	return slog.NewJSONHandler(w, opts)
}

// stderrIsTTY reports whether stderr is an interactive terminal.
func stderrIsTTY() bool {
	return term.IsTerminal(os.Stderr.Fd()) && os.Getenv("TERM") != "dumb"
}

// colorAllowed reports whether ANSI colors may be emitted on stderr.
func colorAllowed() bool {
	return stderrIsTTY() && os.Getenv("NO_COLOR") == ""
}

// setupLogging configures slog for stderr and, when logFile is set, adds a
// rotating JSON file sink that receives the same events (tee). The returned
// writer must be closed by the caller.
func setupLogging(logLevel string, logFile string, maxSizeMB, maxBackups int) (*logging.RotatingWriter, error) {
	level := resolveLogLevel(logLevel)
	slog.SetDefault(slog.New(buildHandler(level, os.Stderr, stderrIsTTY(), colorAllowed())))

	logFile = resolveLogFile(logFile)
	if logFile == "" {
		return nil, nil
	}

	w, err := logging.NewRotatingWriter(logFile, int64(maxSizeMB)<<20, maxBackups)
	if err != nil {
		return nil, err
	}
	fileHandler := buildHandler(level, w, false, false)
	slog.SetDefault(slog.New(slog.NewMultiHandler(slog.Default().Handler(), fileHandler)))
	return w, nil
}

// resolveLogFile returns the log file path, falling back to the
// NOTEBRAIN_LOG_FILE env var when the flag and config file leave it unset.
func resolveLogFile(logFile string) string {
	if logFile == "" {
		return os.Getenv("NOTEBRAIN_LOG_FILE")
	}
	return logFile
}

// hyperlinkSupported returns true if the terminal supports OSC 8 hyperlinks
// and the user has not disabled them.
func hyperlinkSupported() bool {
	if os.Getenv("NO_HYPERLINKS") != "" {
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
