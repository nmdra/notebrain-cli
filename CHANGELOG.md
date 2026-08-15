# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v2.13.0] - 2026-08-15

### Added
- **Reference Listing**: `notebrain refs <note>` lists a note's local attachments (images, PDFs, archives, …) and external website links, filterable with `--only-images`, `--only-pdf`, `--only-other`, and `--only-external-links`. Broken links are hidden by default and surfaced with `--include-missing` (`feat(cmd)`).

### Changed
- **Refs kind filters renamed**: `refs` filters are now `--only-images`/`--only-pdf`/`--only-other`/`--only-external-links` with "limit to" semantics; no flags = all kinds, combine to union (`refactor(cmd)`).
- **PDF flag unified**: `ingest` now uses `--with-pdf` (matching `search`/`boosted`); the config key is `with-pdf` (`refactor(cmd)`).
- **Hidden depth flag**: `--candidate-chunks` is now the single flag for `hidden --deep` depth (`refactor(cmd)`).
- **Search exclude flag**: `--exclude-note` is now `--exclude-notes` (`refactor(cmd)`).
- **Get positional**: `get` positional argument renamed `<SLUG>` → `<NOTE>` in documentation and conventions (`refactor(cmd)`).
- **TSV parity**: `search` TSV header `slug` → `note_slug`; `get` TSV fields escaped; scores at 4 decimal places in TSV, matching JSON (`fix(output)`).
- **Refs TSV**: external-link rows now emit `false` in the `missing` column instead of a blank cell (`fix(output)`).
- **JSONPath envelope parity**: `get` and `stats` apply `--jsonpath` to their full command envelope; `$.command` now works, and `get` paths shift from `$.note_slug` to `$.note.note_slug` (`fix(output)`).
- **Refs text styling**: `refs` text output now matches the house style — note title header, per-kind colored `[kind]` chips (images, PDFs, other, external links), amber `(missing)` markers, clickable links (obsidian:// for attachments, the URL for external links), vault-relative paths instead of absolute ones, and terminal-width truncation (`feat(cmd)`).

### Deprecated

_None._

### Removed
- `hidden --top-k` (alias of `--candidate-chunks`) — breaking; use `--candidate-chunks`, which now defaults to `3`.
- `refs --images`/`--pdf`/`--other`/`--external-links` — breaking; use `--only-*`.
- `ingest --enable-pdf` and config key `enable-pdf` — breaking; use `--with-pdf` (`with-pdf`).
- `search --exclude-note` — breaking; use `--exclude-notes`.
- `--debug` (and config key `debug`) — breaking; use `--log-level=debug` (`log-level = "debug"`).
- `hide-tags` config key — breaking; use `show-tags`.

## [v2.12.0] - 2026-08-02

### Added
- **Unified JSON Envelope**: `get` and `stats` JSON output is now wrapped in the same command envelope used by `search` (`feat(output)`).
- **Stdin Query Support**: `search` now accepts the query from stdin when piped, enabling shell pipelines (`feat(search)`).
- **Setup Wizard Enhancements**: The `init` wizard now prefills detected defaults, validates input, and previews the planned configuration before applying it (`feat(init)`).
- **Shared Tag Flags**: Tags are now aligned with the shared query flags, and `--top-k` is clarified as a hidden flag (`feat(cmd)`).
- **Ingest Progress**: Ingestion reports periodic info-level progress, logs embedding model warmup, and warns after the validate step (`feat(ingest)`).
- **Titled Help Sections**: `--help` output now groups flags under titled sections and fixes placeholder rendering (`feat(help)`).
- **TTY-Aware Colors**: Colors are disabled on non-TTY stdout, and the PDF emoji was dropped from output (`feat(ui)`).
- **Hardened Logging**: Improved logging and error handling across the CLI (`feat(cli)`).

### Fixed
- **Hidden `--deep`**: Unlinked notes are no longer hidden when `--deep` is passed (`fix(store)`).
- **Exit Codes**: Missing query and missing vault path now exit with code 2 (`fix(cli)`).
- **TSV Output**: TSV records stay on one line, and `stats` supports TSV output (`fix(output)`).
- **Recovery Hints**: Error messages now include recovery hints, and the reset confirmation accepts `y` (`fix(errors)`).
- **Log Noise**: The embedding model warmup log is demoted to debug level (`fix(cmd)`).

## [v2.11.0] - 2026-08-01

### Added
- **Shell Completion**: Added tab completion for bash, zsh, and fish covering subcommands, flags, enum values, and live note slugs from the index via a hidden `suggest-notes` command (`feat(cmd)`).
- **ChromaDB Corruption Detection**: `notebrain doctor` now detects corrupted databases and reports them clearly (`feat(cmd)`).
- **Configurable `--log-level` Flag**: Added `--log-level` back as a configurable CLI flag with TOML support, replacing the binary debug flag (`fix(cli)`).
- **Code Block Metadata**: Ingestion now persists whether a chunk contains fenced code blocks, enabling the `--has-code` search filter (`feat(ingest)`).
- **Recursive Glob Ingestion**: Ingestion now supports recursive `**` glob patterns via doublestar (`fix(ingest)`).
- **Documentation**: Rewrote the wiki in Simplified Technical English and documented shell completion setup, attachment embeds, JSONPath exit codes, and chunk schema v5 (`docs(wiki)`).
- **Agent Skill Updates**: Corrected the `file_path` default claim, added search filter flags, and added pre-flight checks to the NoteBrain agent skill and `notebrain-chat` agent definition (`docs(skill)`).

### Changed
- **Store Robustness**: Guarded paginated `Get` helpers against nil responses, clamped search limits, aligned `MatchedQueries`, and dropped a redundant filter block (`fix(store)`).
- **Parser Cleanup**: Replaced the wikilink if-else chain with a switch and snapped chunk boundaries out of placeholder tokens (`refactor(parser)`).
- **Embedder Simplification**: Dropped a pointless goroutine wrapper in the embedding path (`refactor(embedder)`).

### Removed
- **Dead Code**: Removed the unused `obsidian` package, legacy pdfextract files, the dead tesseract OCR path, and the unreachable ndjson output format (`chore`, `refactor(pdfextract)`, `fix(cmd)`).

### Fixed
- **Store Reliability**: Deterministic link vectors and metadata error propagation, written tags capped to the queryable range, swallowed query errors propagated, FFI-cap warnings emitted once per command, and cleanup/graph queries paginated within FFI limits (`fix(store)`).
- **Parser**: Preserved unicode in slugs, allowlisted attachment extensions, and cleaned image-embed noise from chunk text and the link index (`fix(parser)`).
- **Ingestion**: Counted failed files in progress reporting, stored overlap-free chunk text while embedding full text, and preserved the note index when all chunks of a note are filtered (`fix(ingest)`).
- **PDF & LLM Parsing**: Honored model-prefix backends with mismatch warnings, capped `Retry-After` handling, split PDF pages on rune boundaries within token budget, and sized the WASM pool to workers (`fix(llmparse)`, `fix(pdfextract)`).
- **CLI**: Propagated JSONPath errors so all commands exit non-zero and resolved an ineffectual assignment in logger setup (`fix(cmd)`).
- **Configuration**: Resolved normalized TOML key collisions deterministically (`fix(configfile)`).

## [v2.10.0] - 2026-07-29

### Added
- **PDF Ingestion Graceful Fallback**: Implemented automatic fallback when `--enable-pdf` is set but LLM API keys are missing, preserving existing indexed PDFs in ChromaDB without crashing (`feat(ingest)`).
- **Store Note Metadata API**: Introduced `GetNoteMetadata` returning `NoteMeta` struct (file type + content hash) to preserve note metadata and deprecated `GetNoteHashes` (`feat(store)`).
- **Ingestion Progress & Context Window**: Added `--context-window` CLI flag, structured per-file progress logging, and ingestion failure reporting (`feat(ingest)`).
- **LLM Backend Detection & Retry Backoff**: Refactored backend auto-detection supporting OpenRouter, DeepSeek, OpenAI, Gemini, and Ollama with exponential retry backoff (`feat(llmparse)`).
- **PDF Ingestion Guide**: Added comprehensive Wiki guide covering PDF ingestion configuration, LLM backend setup, and graceful fallbacks (`docs(wiki)`).

### Changed
- **LLM Parser Architecture & Ollama Path**: Reused `http.Client` instances across retries/chunks, modularized chunk conversion methods, and normalized Ollama host `/v1` endpoint path handling (`refactor(llmparse)`).

### Fixed
- **Stale Subfolder Wikilinks & PDF Graph Display**: Fixed subfolder PDF fallback target lookup in `processOutgoingLinks` and surfaced `file_type` in graph query outputs for proper `📄 [PDF]` prefix rendering (`fix(store)`).
- **API Max Retries**: Reduced maximum LLM API retry attempts from 5 to 3 (`fix(llmparse)`).

## [v2.9.0] - 2026-07-27

### Added
- **Unified Debug Flag (`--debug`)**: Added a single `--debug` CLI flag and `debug` TOML setting to toggle debug-level logging to stderr (`feat(cli)`).
- **Inverted Tag Flag (`--show-tags`)**: Renamed `--hide-tags` to `--show-tags` (default `false`) for cleaner positive-boolean semantics in CLI output (`feat(cli)`).
- **TOML Deprecated Key Migration**: Implemented automatic resolution of deprecated keys (such as `hide-tags` → `show-tags`) in `TOMLResolver` with non-fatal warning logs for full backward compatibility (`feat(config)`).

### Changed
- **Streamlined Configuration Template**: Reduced `config.example.toml` from 118 lines down to ~35 lines, exposing only core essential settings (`refactor(config)`).
- **Exposed Real Ingest Defaults**: Ingestion flags `--min-chunk-words` (10), `--chunk-size` (800), and `--chunk-overlap` (100) now expose their actual numerical default values directly in `--help` output (`refactor(cli)`).
- **Auto-Detected OCR for PDFs**: OCR via Tesseract is now automatically detected whenever `--enable-pdf` is active and `tesseract` is present in `$PATH`, removing the need for a separate `--enable-ocr` flag (`refactor(ingest)`).
- **Hyperlinks via Environment Variable**: Removed `--no-hyperlinks` flag in favor of standard `$NO_HYPERLINKS` environment variable (`refactor(cli)`).

### Removed
- **Redundant CLI Flags**: Cleaned up 8 redundant CLI flags (`--split`, `--split-by`, `--verbose`, `--no-hyperlinks`, `--skip-attachments`, `--enable-ocr`, `--log-format`, `--log-level`) to simplify the CLI surface area (`refactor(cli)`).

## [v2.8.0] - 2026-07-27

### Added
- **LLM-Based PDF Extraction**: Replaced the heuristic `pdf2md` parser with an LLM-powered extraction pipeline using OpenRouter and DeepSeek APIs to convert messy PDF text into perfectly formatted Markdown.
- **LLM Model Flag**: Added `--llm-model` CLI flag to specify which model to use for PDF text parsing. Any model prefix not explicitly `deepseek-` is routed through OpenRouter by default.

### Changed
- **Graceful PDF Failures**: The ingest pipeline now logs a warning and gracefully skips a PDF if the extraction or LLM API fails, preventing it from crashing the entire vault ingestion process.
- **API Key Sanitization**: Extraneous spaces or quotes are now automatically stripped from API keys loaded via environment variables to prevent HTTP 401 Unauthorized errors.

### Removed
- **Heuristic pdf2md Parser**: Completely removed the old `pdf2md` custom parsing logic as it produced noisy and poorly structured content.

## [v2.7.2] - 2026-07-25

### Fixed
- **Linter & Code Quality Warnings**: Resolved variable shadowing, unused parameters, redundant conversions, whitespace, and gosec linter warnings across `cmd`, `configfile`, `ingest`, `obsidian`, `parser`, and `store` packages (`fix`).
- **golangci-lint Schema & Linter Compliance**: Resolved invalid `goconst.ignore-tests` property in `.golangci.yml` schema and replaced literal string occurrences (`"json"`, `"tsv"`, `"task_list"`) with package constants (`fix(ci)`).

### Changed
- **GitHub Actions Linting**: Extracted `golangci-lint` step into a reusable workflow (`lint.yml`) executing on all branches, upgraded to version `2.12.2`, and configured release job dependency (`ci`).
- **Command Package String Constants**: Replaced repeated string format literals with package-level constants to improve code maintainability (`refactor(cmd)`).
- **Import Grouping Formatting**: Standardized and reformatted import groupings across internal packages and `cmd` (`style`).
- **Dependencies & Vendor Tree**: Upgraded transitive dependencies and refreshed the vendor tree (`build(deps)`).
- **CI & Pre-Commit Configuration**: Configured `golangci-lint` with test exclusions and updated `lefthook` pre-commit hooks (`ci`).

## [v2.7.1] - 2026-07-23

### Fixed
- **Hierarchical Tag Search Duplication**: Fixed an issue where `tags --children` would return duplicate notes across pagination pages by implementing cross-page deduplication (`fix(store)`).
- **Terminal Hyperlink Restoration**: Restored OSC 8 terminal hyperlinks by setting `--show-file-path` to `true` by default. When explicitly set to `false`, the instructional click footer is now properly hidden (`fix(cli)`).

### Changed
- **Pre-commit Hooks**: Added `modernize` to the `lefthook` configuration for automated codebase modernization (`ci`).
- **Documentation**: Updated default flags in agent SKILL and Wiki references to reflect `--show-file-path=true` (`docs`).

## [v2.7.0] - 2026-07-23

### Added
- **Direct Tag Search**: Inverted the `tags` command so direct tag search is now the default behavior, returning all notes containing a specified tag. Shared tag discovery is available via `--shared` (`feat(cmd)`).
- **Absolute File Path Output (`--show-file-path`)**: Added `--show-file-path` CLI flag and `show-file-path` TOML option to optionally include absolute file paths in outputs (`feat(cli)`).
- **Tag Lowercasing Standardization**: Extracted tags from frontmatter and inline elements are standardized to lowercase to guarantee consistent case-insensitive database matching (`feat(parser)`).

### Fixed
- **ChromaDB FFI Buffer Overflow Prevention**: Implemented chunked/paginated querying (`ffiSafePageSize = 200`, `ffiSafeSemanticLimit = 100`, `paginatedGetMetadatas`) across all ChromaDB FFI operations (`TagSearch`, `semanticSearch`, `Backlinks`, `HiddenConnectionsDeep`, `GetNote`, `PopulateContext`, graph traversal, and ingestion cleanup), preventing 1 MiB FFI buffer overflow crashes on large vaults (`fix(store)`).
- **Core Implementation Fixes**: Resolved edge-case issues in `MinChunkWords` configuration parsing, database `Stats` chunk/link pagination, and unused configuration settings (`fix`).

### Changed
- **Streamlined CLI Output & TUI Deprecation**: Removed interactive TUI dependencies (`bubbletea`, `bubbles`, `lipgloss`) and simplified JSON output flags by removing redundant `--compact` and `--ndjson` options to keep CLI focus on fast, clean machine output (`refactor(cli)`).
- **AI Models and Thinking Modes Documentation**: Updated README and Wiki with recommended AI models (DeepSeek V4 Flash, Tencent hy3, Gemini Flash 3.6) and adjusted thinking mode configurations (`docs`).
- **Animated SVG Banner**: Redesigned README hero section with an animated SVG banner featuring pulsing graph nodes (`docs`).

## [v2.6.0] - 2026-07-20

### Added
- **Rich Text & Formatting Preservation**: Added a custom `chunkRenderer` to reconstruct inline text chunks while preserving formatting like emphasis, strikethrough, code spans, inline math, and external links in rich-text mode (`feat(parser)`).
- **Expanded Goldmark Extensions (Latex, Mermaid, Footnotes)**: Initialized and extended the Goldmark parser to parse inline LaTeX math, Mermaid diagrams (with plain-text and diagram fallbacks), and footnotes during document traversal (`feat(parser)`).
- **Metadata Transformer for Tags and Links**: Implemented a unified AST walker context (`metadataTransformer`) to gather tags (including frontmatter tags) and outgoing wikilinks early in parsing (`feat(parser)`).

### Changed
- **Deduplicated AST Walking & Simplified Renderer**: Consolidated AST traversal logic, separated block and inline rendering routines into clean components, and enhanced test coverage of inline elements (`refactor(parser)`).
- **Reduced Cyclomatic Complexity**: Restructured and simplified core parsing functions and control paths across core packages to improve maintainability (`refactor`).
- **Eliminated Dead Code**: Cleaned up obsolete metadata structures and dead code branches (`refactor`).
- **Agent Skill Optimization**: Heavily refined `SKILL.md`, `references/flags.md`, and `references/schema.md` to improve instruction clarity, format consistency (e.g. `--compact` by default), command maps, boundaries, and pre-flight verifications (`docs(skill)`).

### Performance
- **Allocation-Free Iteration**: Switched from line slice allocations to Go 1.23 `strings.SplitSeq` sequence iteration in list, blockquote, and code-block rendering paths to reduce memory overhead (`perf(parser)`).

## [v2.5.0] - 2026-07-17

### Added
- **Suppress text from search results (`--include-text=false`)**: Integrated a toggle parameter `includeText` throughout the ChromaDB query layer (`internal/store`) to suppress loading the `text` field at the storage query level, and cleared `r.Text` during CLI output filtering (`cmd/print`) to ensure lean payload formatting and conserve model tokens (`feat(store,cmd)`).

### Changed
- **Decluttered and Restructured Agent Skills**: Redesigned the Progressive Retrieval Workflow inside `.agents/skills/notebrain/SKILL.md` and `wiki/Notebrain-Chat.md` to prevent duplication, outlining a clear 4-step progressive search strategy with dedicated example commands and a scannable quick command map (`docs(skill,wiki)`).

## [v2.4.0] - 2026-07-15

### Added
- **Token-Efficient Compact JSON Output (`--compact`)**: Added `--compact` CLI flag and `compact = true` TOML configuration option (`~/.notebrain/config/config.toml`) to strip redundant envelope properties (`command`) and result properties (`file_path`) across `json` and `ndjson` formats, reducing JSON token consumption by ~40–50% when ingested by LLMs (`feat(cli)`).
- **Automatic Quiet Mode for Machine Formats (`--quiet`)**: Non-interactive command outputs (`--format=json`, `tsv`, `ndjson`, or `--jsonpath`) now automatically activate quiet mode (`embedder.WithQuiet`), suppressing the embedder loading spinner and background log noise so that AI agents and shell scripts receive 100% clean, uncorrupted machine data (`feat(embedder)`).

### Changed
- **LLM-Optimized Result Formatting**: Standardized similarity scores (`score`) to round to 4 decimal places across JSON envelopes, separated raw query strings (`rawQuery`) from TUI headers, and updated `notebrain search` / `hidden` / `boosted` context windowing (`--context-window N`) to exclude the matched chunk (`text`) itself from the `context` array to prevent token redundancy (`feat(cli,store)`).
- **AI Agent Skill & Reference Documentation**: Expanded the `notebrain-assistant` skill (`SKILL.md`, `references/flags.md`, and `references/schema.md`) with comprehensive guidance on `--compact` mode, automatic `WithQuiet` suppression, 4-decimal score precision, and non-redundant context windows (`docs(skill)`).

## [v2.3.0] - 2026-07-12

### Added
- **Deep Connection Discovery (`--deep`)**: Implemented section-level chunk-by-chunk hidden connections parsing, matching specific target sections (`§ <Heading>`) and similarity thresholds rather than only comparing whole notes (`feat(hidden)`).
- **Already-Linked Filtering Control (`--include-linked`)**: Added option to include notes that are already directly or indirectly linked in the hidden connections output, while strictly excluding self-references (`feat(cli)`).
- **Dynamic Section Header Reconstruction**: Reconstructed note content via `notebrain get` now automatically prepends the matching section header metadata (`### Heading`) for all constituent chunks to ensure structural and markdown continuity (`feat(store)`).
- **Two-Tier Similarity Filtering**: Enforced dual similarity thresholds across query pipelines to deliver highly targeted search results (`feat(store)`).
- **Context-Aware Empty Result Guidance**: Display command-specific discovery tips in terminal `text` output (e.g., suggesting re-ingestion, increasing hops, or enabling `--include-linked`) when query results are empty, while ensuring machine-readable formats (`json`, `tsv`, `ndjson`, `--jsonpath`) remain perfectly clean (`feat(cli)`).
- **Universal Note Slug Resolution**: Enabled fallback slug resolution to match notes across the CLI by either slug or relative path interchangeably (`feat(cli)`).

### Fixed
- **Markdown AST Ingestion Preservation**: Updated parsing to preserve structural markdown markup (unordered/ordered list formats, task checkboxes, callouts, blockquotes, and GFM tables with separator rows) verbatim across chunk divisions (`fix(parser)`).
- **Link Target Path Canonicalization**: Strips `#heading` anchors and resolves nested folder paths to actual canonical vault paths, ensuring backlinks and connection traversals match accurately (`fix(store)`).
- **Large Vault Stats Query Paginated Scan**: Paginated the database stats retrieval to prevent JSON buffer overflows on vaults containing tens of thousands of chunks (`fix(store)`).
- **Systemd Service Specifier**: Explicitly set the service unit target in systemd timer files (`fix(systemd)`).

### Changed
- **CLI Output Styling and Help Standardization**: Polished TUI/terminal outputs, lazy-initialized lipgloss styles, and unified Kong flag descriptions (`feat(cli)`).
- **Refactoring & Code Quality**: Modernized `cloneMetaMap` using Go 1.21 `maps.Copy`, extracted unified testing store setup (`newTestStore`), reduced complexity of database testing, and improved test coverage of CLI commands and edge cases (`refactor`).

## [v2.2.0] - 2026-07-07

### Added
- **Multi-Query Split Search**: Query inputs can now be split (by delimiters like commas, semicolons, or pipes) into multiple sub-queries. The search results from all sub-queries are merged, de-duplicated, and boosted based on the number of matching queries, with hit attribution (`[hits: ...]`) shown in text output (`feat(search)`).
- **Tag Suppression Configuration**: Added a `--hide-tags` CLI flag and `hide-tags` TOML configuration key (defaulting to `true`) to suppress tag lists from search output to conserve terminal space and token limits (`feat(config)`).

### Fixed
- **ChromaDB FFI 1 MiB Payload Protection**: Reverted local vendor modifications to keep external dependencies clean, and implemented a local error wrapper (`wrapChromaErr`) to intercept ChromaDB JSON decoding truncation errors (1 MiB FFI limit) with clear mitigation advice. Added an immediate warning in the search CLI when `--top-k >= 4` is specified (`fix(store,cli)`).

### Changed
- **Search Output UX Enhancements**: Improved the formatting and layout of semantic search results, including updated configuration examples (`feat(cmd)`).
- **AI Agent Skill Refactoring**: Extensively restructured the built-in AI assistant skill (`SKILL.md` and `references/`) for progressive disclosure to dramatically reduce token consumption. Documented chunk-based architectures, introduced JSON output schemas, and prohibited blind note fetching in favor of precise context windowing and `--jsonpath` extraction (`docs(skill)`).
- **Documentation Updates**: Updated the command reference, architecture guides, and README to include feature overviews and visual screenshot assets (`docs(wiki,readme)`).

## [v2.1.1] - 2026-07-05

### Added
- **System Architecture & Schema Docs**: Added comprehensive Mermaid architecture diagram and detailed ChromaDB collection schema (`nb_chunks`, `nb_links`) to Wiki (`docs(wiki)`).
- **Dependabot Configuration**: Added automated Go module dependency tracking via Dependabot (`chore(deps)`).

### Fixed
- **Go Modules v2 SIV Compliance**: Updated module path in `go.mod` and all internal import statements to `github.com/nmdra/notebrain-cli/v2`, fixing Go module resolution (`go get`, `pkg.go.dev`) for v2 releases (`refactor(build)`).

### Changed
- **GoReleaser Vendoring**: Configured GoReleaser to use `-mod=vendor` flag and run `go mod vendor` in pre-release hooks (`ci`).
- **README Refinements**: Added creator note and updated OSC 8 terminal support list (`docs(readme)`).
- **Dependency Upgrades**: Updated `charm.land/bubbletea/v2` (v2.0.8), `charm.land/bubbles/v2` (v2.1.1), `charm.land/lipgloss/v2` (v2.0.5), `github.com/pelletier/go-toml/v2` (v2.4.3), and `golang.org/x/sys` (v0.46.0) (`build(deps)`).

## [v2.1.0] - 2026-07-05

### Added
- **Chunk Windowing (`--window`)**: Adjacent context retrieval returns surrounding chunks for richer semantic context (`feat(store,cli)`).
- **Top-K Deduplication**: Search results are now deduplicated per note, returning only the best-scoring chunk per document (`feat(store,query)`).
- **Raw Code Block Preservation**: Code blocks are now preserved verbatim in stored chunk text during parsing and ingestion (`feat(parser,ingest)`).
- **Skip-Attachments & Skip-Phantom Filtering**: New `--skip-attachments` and `--skip-phantom` flags filter attachment files and phantom (non-existent) links from results (`feat(store)`).
- **Structured JSON Logging**: Added `--log-format=json` for machine-readable structured logs via `log/slog`, optimized for headless/cron execution (`feat(log)`).
- **Git-Tag Release Versioning**: `notebrain version` subcommand now reports the build version from git tags (`feat(cli)`).
- **Scheduled Ingestion Templates**: Added systemd timer and cron templates for automated 3-hour ingestion cycles (`feat(automation)`).
- **TOML-Only Configuration**: Removed `.env` support; all configuration is now strictly CLI flags > TOML file with normalized key matching (`feat(config)`).

### Fixed
- **Token-Aware Truncation Guard**: Embedding text is now truncated to the model's max token limit before encoding, preventing silent failures on very large chunks (`fix(ingest)`).
- **TTY Detection for Headless Execution**: Interactive TUI components are now automatically disabled when running under systemd, cron, or non-TTY environments (`fix(tui)`).

### Changed
- **Embedded Persistent Storage Only**: Removed HTTP/standalone ChromaDB server mode; the CLI now strictly uses embedded persistent storage (`refactor(core)`).
- **Decoupled Progress Logging**: Ingestion progress logging moved out of `internal/tui/` into the ingest domain package for cleaner headless operation (`refactor(ingest)`).
- **RWMutex for Concurrent Reads**: Store layer upgraded from `sync.Mutex` to `sync.RWMutex`, allowing concurrent read queries (`refactor(store)`).
- **Parser AST Type Rename**: Removed package-name stutter from AST types and methods for idiomatic Go (`refactor(parser)`).
- **Modern Go Idioms**: Applied optimized string builder writes and modern Go patterns across core packages (`refactor(core)`).

### Performance
- **Memory & String Optimizations**: Reduced allocations and optimized string processing across parser, store, and embedder packages (`perf(core)`).

### Build
- **Go 1.26.4**: Bumped Go version to 1.26.4 (`build(deps)`).

### Tests
- **Embedder & Obsidian Package Tests**: Added comprehensive table-driven unit tests for embedder and obsidian client packages (`test(core)`).

## [v2.0.0] - 2026-06-30

### Breaking Changes
- **Modernized JSON Schema (`snake_case`)**: All machine-readable structured outputs (`--format json`, `tsv`, `ndjson`) have been modernized from PascalCase (`Title`, `Score`, `FilePath`) to clean `snake_case` keys (`title`, `score`, `file_path`, `note_slug`, `tags`, `text`). Consumer automation scripts and AI agents must be updated to reference `snake_case` keys.

### Added
- **AI Agent Command Chaining (`--jsonpath`)**: Integrated `jsonpath` expression evaluation across all query and stats commands (`--jsonpath="$.results[0].note_slug"`). Scalar outputs format as unquoted raw strings and arrays print newline-separated elements, allowing direct shell pipeline integration without external JSON parsers like `jq`.
- **Complete Note Retrieval (`notebrain get`)**: Added a dedicated `get <slug-or-path>` command to retrieve and stitch together all indexed document chunks into the full reconstructed markdown note content.
- **Tag Search & Filtering (`--tag`)**: Added direct tag filtering (`--tag="TagName"`) to `notebrain search` and expanded tag extraction across note metadata.
- **AI Agent Skill Instructions**: Added and documented the built-in `notebrain-assistant` skill (`.agents/skills/notebrain/SKILL.md`) optimized for agentic coding tools.
- **TOML Configuration File Support**: Added support for persisting CLI flags via `~/.notebrain/config/config.toml` along with flags `--respect-exclude` and `--use-editor`.
- **External Editor Integration (`--use-editor`)**: Added ability to open matching notes directly in terminal/GUI editors defined by `$EDITOR` from the interactive TUI.
- **Obsidian Ignore & Attachment Filtering**: Automatically honors Obsidian's `userIgnoreFilters` and `attachmentFolderPath` settings during ingestion when `--respect-exclude` is enabled.

### Fixed
- **HNSW Concurrency & Integrity Bugs**: Implemented batch database writes and serialized chunk deletion/insertion operations to eliminate embedded `hnswlib` assertion failures during high concurrency ingestion and collection reset.

## [v1.1.0] - 2026-06-19

### Added
- **Beautiful TUI Integration**: Upgraded the CLI experience by integrating the `charm.land/bubbles v2` and `lipgloss v2` ecosystem.
- **Interactive Result Browser**: Semantic searches and link traversals now open an interactive terminal UI (TUI) where you can fuzzy-find through results, view scores, and instantly open matched notes in Obsidian using the `Enter` key.
- **Live Ingestion Progress**: Replaced the static progress output with a smooth, live-updating progress bar displaying exactly which files are being processed.

### Changed
- **CLI Framework Migration**: Completely migrated the CLI definition from `cobra` to `kong` for cleaner, declarative, struct-based command definitions, improving maintainability.
- **Safe Pipeline Interruptions**: You can now safely cancel the multi-worker ingestion pipeline at any time by pressing `ctrl+c` (or `q`/`esc` in the TUI). All workers will cleanly abort.

### Fixed
- **Concurrency Crash**: Fixed a critical bug where embedded `hnswlib` (ChromaDB) would crash with a core dump (assertion failure) during concurrent ingestion. Database writes are now safely synchronized with mutexes.
- **Missing FilePaths**: Fixed an issue where the `backlinks`, `connections`, `hidden`, and `tags` commands returned results missing `FilePath` metadata, meaning Obsidian URIs and the new interactive UI open feature now work perfectly for all commands.
- **Zombie Process Leak**: Fixed an issue where opening a note via the terminal leaked zombie processes in the background.
- **Chroma Path Resolution**: Fixed an issue where the `~` character in the database path was evaluated as a literal directory name instead of expanding to the user's home directory.



## [v1.0.0] - 2026-06-18

### Official Release
This marks the official stable release of **NoteBrain CLI v1.0.0**! This release successfully graduates all of our powerful experimental features from the alpha, beta, and release candidate channels into a robust, high-performance production version.

**Highlights of v1.0.0 include:**
- **Local Embedded AI**: Fully embedded `chroma-go` database and local ONNX embedding models right in the binary. No external servers needed.
- **AST-Aware Intelligence**: Goldmark-based markdown parsing for highly accurate, structure-aware semantic chunking.
- **Graph & Semantic Search Combined**: Search by vector similarity, explore wikilink connections (`--hops`), and run Graph-Boosted hybrid queries.
- **Terminal Integration**: Clickable OSC 8 `obsidian://open` terminal hyperlinks integrated natively.
- **Developer Experience**: Dotenv (`.env`) support, 74%+ test coverage, and automated `GoReleaser` pipelines.

## [v1.0.0-rc.1] - 2026-06-18

### Added
- **Goldmark AST-Aware Chunking**: Intelligently chunks markdown sections according to header hierarchies instead of arbitrary character splits, preserving code blocks and structural metadata.
- **Advanced Filtering**: Use `--section`, `--has-code`, and `--has-tasks` flags on searches to filter precisely by document structure.
- **OSC 8 Terminal Hyperlinks**: Automatically renders clickable `obsidian://open` links right in your CLI for seamlessly opening matched chunks inside Obsidian (supported terminals only). Added `--no-hyperlinks` flag to disable.
- **Environment Configuration**: Added `.env` file support (via `godotenv`) to manage global configuration like `OBSIDIAN_VAULT_PATH` and `OBSIDIAN_VAULT_NAME` without repetitive flags.

## [v1.0.0-beta] - 2026-06-18

### Added
- **Content Hashing**: Introduced SHA-256 hashing during the ingest pipeline to safely and instantly skip re-ingesting files that haven't changed.

### Changed
- **Performance**: Greatly improved test coverage (up to 74.1%) across parser, store, and ingest systems.
- **Refactoring**: Stripped over-engineered code: removed `obsidian` package, removed abstract embedder interfaces, and inlined custom sorting functions.

## [v1.0.0-alpha] - 2026-06-18

### Added
- **Embedded ChromaDB Engine**: Fully migrated from DuckDB/pgvector to an embedded `chroma-go` v2 vector database.
- **Local ONNX Embeddings**: Added in-process inference using local ONNX embedding models to vector-encode markdown chunks seamlessly.
- **Wikilink & Tag Graph Processing**: Parses Obsidian wikilinks and frontmatter tags to construct structural graph relationships in vector space.
- **CLI Commands**:
  - `ingest`: Fully concurrent pipeline to parse, chunk, and embed an Obsidian vault.
  - `search`: Semantic vector search for textual matching.
  - `backlinks`: Identifies incoming references to a target note via the structural graph.
  - `connections`: Explores breadth-first structural subgraphs (n-hop traversal).
  - `hidden`: Discovers "hidden" semantic links between unlinked notes based on high semantic proximity.
  - `boosted`: Combines vector similarity with graph connectivity (Graph-Boosted Semantic Search).
  - `tags`: Discovers notes sharing identical frontmatter tags.
  - `stats`: Analyzes current ChromaDB vector storage statistics.
  - `reset`: Completely purges the embedded vector database.
- **Automated CI/CD**: Added `GoReleaser` and GitHub Actions configuration for automated, cryptographically signed binary distribution and SBOM generation.
- **Documentation**: Comprehensive README and `wiki/` documentation covering Installation, Commands, and Architecture.
