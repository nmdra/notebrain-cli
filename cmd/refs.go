/*
Copyright © 2026 nmdra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nmdra/notebrain-cli/v2/internal/ingest"
	"github.com/nmdra/notebrain-cli/v2/internal/parser"
)

// refs kinds. They mirror parser.AttachmentKind but are plain strings so the
// output layer stays independent of the parser package.
const (
	kindImage    = "image"
	kindPDF      = "pdf"
	kindOther    = "other"
	kindExternal = "external-links"
)

type RefsCmd struct {
	Note           string `arg:"" help:"note slug, title, or file path (auto-resolved)" completion-predictor:"note-slug"`
	OnlyImages     bool   `group:"refs" name:"only-images" help:"limit to image attachments (no filter = all kinds; combine filters to union)" default:"false"`
	OnlyPDF        bool   `group:"refs" name:"only-pdf" help:"limit to PDF attachments (no filter = all kinds; combine filters to union)" default:"false"`
	OnlyOther      bool   `group:"refs" name:"only-other" help:"limit to other attachments (video, audio, archives, office docs) (no filter = all kinds; combine filters to union)" default:"false"`
	OnlyExternal   bool   `group:"refs" name:"only-external-links" help:"limit to external website links (URLs) (no filter = all kinds; combine filters to union)" default:"false"`
	IncludeMissing bool   `group:"refs" name:"include-missing" help:"include references whose file is missing from the vault" default:"false"`

	// Deprecated aliases for the kind filters, kept parseable but hidden from
	// --help. Use the --only-* flags instead.
	Images        bool `group:"refs" name:"images" hidden:"" help:"deprecated: use --only-images" default:"false"`
	PDF           bool `group:"refs" name:"pdf" hidden:"" help:"deprecated: use --only-pdf" default:"false"`
	Other         bool `group:"refs" name:"other" hidden:"" help:"deprecated: use --only-other" default:"false"`
	ExternalLinks bool `group:"refs" name:"external-links" hidden:"" help:"deprecated: use --only-external-links" default:"false"`
}

// refEntry is one resolved reference row. Path is absolute for attachments and
// the URL for external links; external rows never carry relative_path.
type refEntry struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path,omitempty"`
	Kind         string `json:"kind"`
	Missing      bool   `json:"missing"`
}

// refsEnvelope is the machine-readable shape of "refs" output.
type refsEnvelope struct {
	Command  string     `json:"command"`
	NoteSlug string     `json:"note_slug"`
	Title    string     `json:"title"`
	Total    int        `json:"total"`
	Refs     []refEntry `json:"refs"`
}

func (c *RefsCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	vaultPath := globals.VaultPath
	if vaultPath == "" {
		return &UsageError{Err: errors.New(vaultPathUsageError)}
	}
	if strings.TrimSpace(c.Note) == "" {
		return &UsageError{Err: fmt.Errorf("%s requires a note slug, title, or file path", groupRefs)}
	}

	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	meta, err := st.GetNoteMeta(ctx, c.Note)
	if err != nil {
		return err
	}
	if strings.HasSuffix(strings.ToLower(meta.FilePath), ".pdf") {
		return fmt.Errorf("note %q is a PDF; refs are listed for markdown notes", c.Note)
	}

	absPath := filepath.Join(vaultPath, meta.FilePath)
	body, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("read note file %q: %w (is --vault-path pointing at the right vault?)", absPath, err)
	}

	entries := c.resolveRefs(vaultPath, meta.FilePath, parser.ExtractReferences(string(body)))
	if !c.IncludeMissing {
		entries = filterExistingRefs(entries)
	}
	entries = filterRefKinds(entries, c)

	env := refsEnvelope{
		Command:  groupRefs,
		NoteSlug: meta.NoteSlug,
		Title:    meta.Title,
		Total:    len(entries),
		Refs:     entries,
	}
	return printRefsFormatted(env, globals)
}

// resolveRefs turns extracted references into resolved entries, deduped by
// resolved absolute path (or exact URL) in first-occurrence order.
func (c *RefsCmd) resolveRefs(vaultPath, noteFilePath string, extracted parser.ExtractedRefs) []refEntry {
	noteDir := filepath.Dir(filepath.Join(vaultPath, filepath.FromSlash(noteFilePath)))
	attachmentFolder := ingest.LoadAttachmentFolderPath(vaultPath)

	var entries []refEntry
	seen := make(map[string]struct{})
	add := func(e refEntry) {
		if _, ok := seen[e.Path]; ok {
			return
		}
		seen[e.Path] = struct{}{}
		entries = append(entries, e)
	}

	for _, ref := range extracted.Refs {
		if ref.Kind == parser.KindExternalLinks {
			add(refEntry{Path: ref.Target, Kind: kindExternal})
			continue
		}
		if entry, ok := c.resolveAttachment(vaultPath, noteDir, attachmentFolder, ref); ok {
			add(entry)
		}
	}
	return entries
}

// resolveAttachment finds the on-disk path of one attachment reference. Wiki
// refs follow Obsidian's search order (note folder, vault root, attachment
// folder); markdown refs resolve note-folder-relative with percent-decoding.
// A reference that escapes the vault is rejected outright (dropped, never
// even listed as missing); one that matches no existing file resolves to its
// first candidate marked missing.
func (c *RefsCmd) resolveAttachment(vaultPath, noteDir, attachmentFolder string, ref parser.Ref) (refEntry, bool) {
	var candidates []string
	switch ref.Source {
	case parser.SrcMarkdown:
		candidates = []string{resolveMarkdownDestination(noteDir, ref.Target)}
	default:
		candidates = wikiCandidates(vaultPath, noteDir, attachmentFolder, ref.Target)
	}
	first := candidates[0]
	if !insideVault(vaultPath, first) {
		return refEntry{}, false
	}
	for _, cand := range candidates {
		if !insideVault(vaultPath, cand) {
			continue
		}
		if _, err := os.Stat(cand); err == nil {
			return refEntry{Path: cand, RelativePath: vaultRelativePath(vaultPath, cand), Kind: string(ref.Kind)}, true
		}
	}
	return refEntry{Path: first, RelativePath: vaultRelativePath(vaultPath, first), Kind: string(ref.Kind), Missing: true}, true
}

// wikiCandidates returns candidate paths for a wiki target in Obsidian
// resolution order. Targets containing "/" start at the vault root; "./"
// resolves relative to the note's folder; bare names search the note folder,
// then the vault root, then the configured attachment folder.
func wikiCandidates(vaultPath, noteDir, attachmentFolder, target string) []string {
	switch {
	case strings.HasPrefix(target, "./"):
		return []string{filepath.Join(noteDir, filepath.FromSlash(strings.TrimPrefix(target, "./")))}
	case strings.Contains(target, "/"):
		return []string{filepath.Join(vaultPath, filepath.FromSlash(target))}
	}
	candidates := []string{filepath.Join(noteDir, filepath.FromSlash(target))}
	if vaultPath != noteDir {
		candidates = append(candidates, filepath.Join(vaultPath, filepath.FromSlash(target)))
	}
	if attachmentFolder != "" {
		candidates = append(candidates, filepath.Join(vaultPath, filepath.FromSlash(attachmentFolder), filepath.FromSlash(target)))
	}
	return candidates
}

// resolveMarkdownDestination decodes a markdown link destination (fragment
// stripped, percent-encoding unescaped) and joins it to the note's folder.
func resolveMarkdownDestination(noteDir, target string) string {
	decoded := target
	if before, _, ok := strings.Cut(target, "#"); ok {
		decoded = before
	}
	if unescaped, err := url.PathUnescape(decoded); err == nil {
		decoded = unescaped
	}
	return filepath.Join(noteDir, filepath.FromSlash(decoded))
}

// insideVault reports whether abs stays within the vault, rejecting `..`
// traversal escapes via filepath.Rel.
func insideVault(vaultPath, abs string) bool {
	rel, err := filepath.Rel(vaultPath, abs)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// vaultRelativePath renders abs as a slash-separated vault-relative path.
func vaultRelativePath(vaultPath, abs string) string {
	rel, err := filepath.Rel(vaultPath, abs)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// filterExistingRefs drops missing rows (broken links are hidden by default).
func filterExistingRefs(entries []refEntry) []refEntry {
	kept := make([]refEntry, 0, len(entries))
	for _, e := range entries {
		if !e.Missing {
			kept = append(kept, e)
		}
	}
	return kept
}

// filterRefKinds keeps rows matching any selected kind flag; no flags select
// every kind. Deprecated aliases (Images/PDF/Other/ExternalLinks) count the
// same as their --only-* replacements.
func filterRefKinds(entries []refEntry, c *RefsCmd) []refEntry {
	if len(entries) == 0 {
		return entries
	}
	filter := refsKindFilterFromCmd(c)
	if !filter.images && !filter.pdf && !filter.other && !filter.external {
		return entries
	}
	kept := make([]refEntry, 0, len(entries))
	for _, e := range entries {
		if filter.keep(e.Kind) {
			kept = append(kept, e)
		}
	}
	return kept
}

// refsKindFilter merges the --only-* flags with their deprecated aliases.
type refsKindFilter struct {
	images, pdf, other, external bool
}

func refsKindFilterFromCmd(c *RefsCmd) refsKindFilter {
	return refsKindFilter{
		images:   c.OnlyImages || c.Images,
		pdf:      c.OnlyPDF || c.PDF,
		other:    c.OnlyOther || c.Other,
		external: c.OnlyExternal || c.ExternalLinks,
	}
}

func (f refsKindFilter) keep(kind string) bool {
	switch kind {
	case kindImage:
		return f.images
	case kindPDF:
		return f.pdf
	case kindOther:
		return f.other
	case kindExternal:
		return f.external
	}
	return false
}

// printRefsFormatted renders a refs envelope to stdout based on the requested
// format. JSONPath extraction applies to the envelope when requested.
func printRefsFormatted(env refsEnvelope, globals *Globals) error {
	return printRefsFormattedToWriter(os.Stdout, env, globals)
}

func printRefsFormattedToWriter(w io.Writer, env refsEnvelope, globals *Globals) error {
	if globals.JSONPath != "" {
		return printJSONPathResultToWriter(w, env, globals.JSONPath)
	}

	switch globals.Format {
	case formatJSON:
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	case formatTSV:
		_, _ = fmt.Fprintln(w, "path\tkind\tmissing\trelative_path")
		for _, r := range env.Refs {
			missing := strconv.FormatBool(r.Missing)
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tsvEscape(r.Path), r.Kind, missing, tsvEscape(r.RelativePath))
		}
		return nil
	default: // "text"
		initStyles()
		title := env.Title
		if title == "" {
			title = env.NoteSlug
		}
		_, _ = fmt.Fprintln(w, headerStyle.Render(title))

		if len(env.Refs) == 0 {
			_, _ = fmt.Fprintln(w, hintStyle.Render("  No references found"))
			return nil
		}

		termWidth := getTerminalWidth()
		useLinks := hyperlinkSupported() && globals.ShowFilePath

		for _, r := range env.Refs {
			chip := refKindChipStyle(r.Kind).Render("[" + r.Kind + "]")

			// Text mode shows vault-relative paths (no base-vault clutter);
			// external links keep their URL, which is the path itself.
			display := r.Path
			if r.Kind != kindExternal && r.RelativePath != "" {
				display = r.RelativePath
			}

			path := display
			if useLinks {
				switch r.Kind {
				case kindExternal:
					path = hyperlink(true, r.Path, display)
				default:
					if r.RelativePath != "" {
						path = hyperlink(true, ObsidianURI(globals.VaultName, r.RelativePath), display)
					}
				}
			}
			line := fmt.Sprintf("%s %s", chip, path)
			if r.Missing {
				line += " " + warnBoldStyle.Render("(missing)")
			}
			if termWidth > 0 && ansi.StringWidth(line) > termWidth {
				line = ansi.Truncate(line, termWidth, "…")
			}
			_, _ = fmt.Fprintln(w, line)
		}

		if useLinks {
			_, _ = fmt.Fprintln(w, "\n  "+extraStyle.Render("(Ctrl+click / Cmd+click a reference to open it)"))
		}
		return nil
	}
}

// refKindChipStyle returns the style for a ref kind chip in text output:
// images use the accent, PDFs the blue tag used by search, other
// attachments the muted gray, and external links the bold label.
func refKindChipStyle(kind string) lipgloss.Style {
	initStyles()
	switch kind {
	case kindImage:
		return titleStyle
	case kindPDF:
		return pdfTagStyle
	case kindOther:
		return metaStyle
	default: // kindExternal
		return labelStyle
	}
}
