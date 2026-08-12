package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// writeRefsTestVault creates a temp vault with the standard fixture note and
// returns the vault root. The note is Notes/router.md:
//
//	![[cover.png]] [[assets/arch.png]] [[./local.png]] [modes](Router%20Modes.webp)
//	[[att.pdf]] [[a.png]] [a](a.png) [broken](broken.png) [ext](https://example.com/docs)
//	[[https://links.example.com]]
//
// It advertises the Obsidian attachment folder 99.Storage-Shed/Attachments.
func writeRefsTestVault(t *testing.T) string {
	t.Helper()
	vaultDir := t.TempDir()
	files := map[string]string{
		".obsidian/app.json":                  `{"attachmentFolderPath": "99.Storage-Shed/Attachments"}`,
		"Notes/router.md":                     "![[cover.png]] [[assets/arch.png]] [[./local.png]] [modes](Router%20Modes.webp)\n[[att.pdf]] [[a.png]] [a](a.png) [broken](broken.png) [ext](https://example.com/docs)\n[[https://links.example.com]]",
		"Notes/cover.png":                     "png",
		"Notes/Router Modes.webp":             "webp",
		"Notes/local.png":                     "png",
		"Notes/a.png":                         "png",
		"assets/arch.png":                     "png",
		"99.Storage-Shed/Attachments/att.pdf": "pdf",
	}
	for path, content := range files {
		full := filepath.Join(vaultDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return vaultDir
}

func refsTestGlobals(vaultDir string) *Globals {
	return &Globals{Ctx: context.Background(), VaultPath: vaultDir}
}

func TestRefsText(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{
		NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md",
	}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router"}).Run(refsTestGlobals(vaultDir)); err != nil {
			t.Errorf("Run: %v", err)
		}
	})

	wantLines := []string{
		"[image] " + filepath.Join(vaultDir, "Notes", "cover.png"),
		"[image] " + filepath.Join(vaultDir, "assets", "arch.png"),
		"[image] " + filepath.Join(vaultDir, "Notes", "local.png"),
		"[image] " + filepath.Join(vaultDir, "Notes", "Router Modes.webp"),
		"[pdf] " + filepath.Join(vaultDir, "99.Storage-Shed", "Attachments", "att.pdf"),
		"[image] " + filepath.Join(vaultDir, "Notes", "a.png"),
		"[external-links] https://example.com/docs",
		"[external-links] https://links.example.com",
	}
	for _, want := range wantLines {
		if !strings.Contains(out, want) {
			t.Errorf("text output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "broken.png") {
		t.Errorf("missing reference shown without --include-missing:\n%s", out)
	}
	idxs := make([]int, len(wantLines))
	for i, want := range wantLines {
		idxs[i] = strings.Index(out, want)
	}
	for i := 1; i < len(idxs); i++ {
		if idxs[i] < idxs[i-1] {
			t.Errorf("output not in first-occurrence order: %q before %q", wantLines[i-1], wantLines[i])
		}
	}
}

func TestRefsFilters(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{
		NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md",
	}}
	withFakeStore(t, fs)

	tests := []struct {
		name    string
		cmd     RefsCmd
		include []string
		exclude []string
	}{
		{
			name:    "images only",
			cmd:     RefsCmd{Note: "router", Images: true},
			include: []string{filepath.Join(vaultDir, "Notes", "cover.png")},
			exclude: []string{"att.pdf", "https://example.com/docs", "[external-links]"},
		},
		{
			name:    "pdf only",
			cmd:     RefsCmd{Note: "router", PDF: true},
			include: []string{filepath.Join(vaultDir, "99.Storage-Shed", "Attachments", "att.pdf")},
			exclude: []string{"cover.png", "https://example.com/docs"},
		},
		{
			name:    "external links only",
			cmd:     RefsCmd{Note: "router", ExternalLinks: true},
			include: []string{"[external-links] https://example.com/docs", "[external-links] https://links.example.com"},
			exclude: []string{"cover.png", "att.pdf", "localhost", ".png"},
		},
		{
			name:    "combined or",
			cmd:     RefsCmd{Note: "router", Images: true, PDF: true},
			include: []string{filepath.Join(vaultDir, "Notes", "cover.png"), filepath.Join(vaultDir, "99.Storage-Shed", "Attachments", "att.pdf")},
			exclude: []string{"https://example.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := captureStdout(t, func() {
				if err := tt.cmd.Run(refsTestGlobals(vaultDir)); err != nil {
					t.Errorf("Run: %v", err)
				}
			})
			for _, want := range tt.include {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			for _, notWant := range tt.exclude {
				if strings.Contains(out, notWant) {
					t.Errorf("output contains excluded %q:\n%s", notWant, out)
				}
			}
		})
	}
}

func TestRefsMissingHiddenUnlessFlagged(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md"}}
	withFakeStore(t, fs)

	missingPath := filepath.Join(vaultDir, "Notes", "broken.png")

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router"}).Run(refsTestGlobals(vaultDir)); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if strings.Contains(out, brokenPNG) {
		t.Errorf("missing reference visible by default:\n%s", out)
	}

	out = captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router", IncludeMissing: true}).Run(refsTestGlobals(vaultDir)); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, missingPath) || !strings.Contains(out, "(missing)") {
		t.Errorf("--include-missing should show the broken reference marked missing:\n%s", out)
	}
}

// brokenPNG names the reference that does not exist on disk in the fixture.
const brokenPNG = "broken.png"

func TestRefsMarkdownTraversalEscapeIsMissing(t *testing.T) {
	vaultDir := t.TempDir()
	noteDir := filepath.Join(vaultDir, "Notes")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noteDir, "escape.md"),
		[]byte("[x](../../secret.pdf)"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "escape", Title: "Escape", FilePath: "Notes/escape.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "escape", IncludeMissing: true}).Run(refsTestGlobals(vaultDir)); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "(missing)") {
		t.Errorf("traversal escape should be rejected and marked missing:\n%s", out)
	}
}

func TestRefsCrossSyntaxDedupe(t *testing.T) {
	vaultDir := t.TempDir()
	noteDir := filepath.Join(vaultDir, "Notes")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[[a.png]] and [x](a.png) and https://example.com and [y](https://example.com)"
	if err := os.WriteFile(filepath.Join(noteDir, "dedupe.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noteDir, "a.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "dedupe", Title: "Dedupe", FilePath: "Notes/dedupe.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		err := (&RefsCmd{Note: "dedupe"}).Run(&Globals{Ctx: context.Background(), VaultPath: vaultDir, Format: formatJSON})
		if err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if want := `"total": 2`; !strings.Contains(out, want) {
		t.Errorf("expected deduped total 2, got:\n%s", out)
	}
	if n := strings.Count(out, filepath.Join(vaultDir, "Notes", "a.png")); n != 1 {
		t.Errorf("a.png listed %d times, want 1:\n%s", n, out)
	}
	if n := strings.Count(out, "https://example.com"); n != 1 {
		t.Errorf("url listed %d times, want 1 (deduped):\n%s", n, out)
	}
}

func TestRefsJSON(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router"}).Run(&Globals{Ctx: context.Background(), VaultPath: vaultDir, Format: formatJSON}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	for _, want := range []string{
		`"command": "refs"`,
		`"note_slug": "router"`,
		`"title": "Router"`,
		`"total": 8`,
		`"relative_path": "Notes/cover.png"`,
		`"kind": "external-links"`,
		`"missing": false`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %s:\n%s", want, out)
		}
	}
	if strings.Contains(out, `"relative_path": "https://`) {
		t.Errorf("external URL must not carry relative_path:\n%s", out)
	}
}

func TestRefsTSV(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router"}).Run(&Globals{Ctx: context.Background(), VaultPath: vaultDir, Format: formatTSV}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "path\tkind\tmissing\trelative_path" {
		t.Errorf("tsv header = %q", lines[0])
	}
	first := filepath.Join(vaultDir, "Notes", "cover.png")
	if want := fmt.Sprintf("%s\timage\tfalse\tNotes/cover.png", first); lines[1] != want {
		t.Errorf("tsv row = %q, want %q", lines[1], want)
	}
	last := lines[len(lines)-1]
	if want := "https://links.example.com\texternal-links\t\t"; last != want {
		t.Errorf("external tsv row = %q, want %q", last, want)
	}
}

func TestRefsJSONPath(t *testing.T) {
	vaultDir := writeRefsTestVault(t)
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "router", Title: "Router", FilePath: "Notes/router.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "router"}).Run(&Globals{Ctx: context.Background(), VaultPath: vaultDir, JSONPath: "$.refs[*].path"}); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	want := filepath.Join(vaultDir, "Notes", "cover.png")
	if !strings.Contains(out, want) {
		t.Errorf("jsonpath output missing %q:\n%s", want, out)
	}
	if !strings.Contains(out, "https://example.com/docs") {
		t.Errorf("jsonpath output missing external url:\n%s", out)
	}
}

func TestRefsEmptyResult(t *testing.T) {
	vaultDir := t.TempDir()
	noteDir := filepath.Join(vaultDir, "Notes")
	if err := os.MkdirAll(noteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noteDir, "plain.md"), []byte("just a note with no references"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "plain", Title: "Plain", FilePath: "Notes/plain.md"}}
	withFakeStore(t, fs)

	out := captureStdout(t, func() {
		if err := (&RefsCmd{Note: "plain"}).Run(refsTestGlobals(vaultDir)); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	if !strings.Contains(out, "No references found") {
		t.Errorf("text mode should report no references:\n%s", out)
	}
}

func TestRefsPDFNoteError(t *testing.T) {
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "guide", Title: "Guide", FilePath: "Notes/guide.pdf"}}
	withFakeStore(t, fs)

	vaultDir := t.TempDir()
	err := (&RefsCmd{Note: "guide"}).Run(refsTestGlobals(vaultDir))
	if err == nil || !strings.Contains(err.Error(), "is a PDF") {
		t.Errorf("expected PDF-note error, got %v", err)
	}
}

func TestRefsNoteFileMissingError(t *testing.T) {
	vaultDir := t.TempDir()
	fs := &fakeStore{noteMeta: &store.NoteContent{NoteSlug: "ghost", Title: "Ghost", FilePath: "Notes/ghost.md"}}
	withFakeStore(t, fs)

	err := (&RefsCmd{Note: "ghost"}).Run(refsTestGlobals(vaultDir))
	if err == nil || !strings.Contains(err.Error(), "--vault-path") {
		t.Errorf("expected missing-file error hinting --vault-path, got %v", err)
	}
}

func TestRefsUsageErrors(t *testing.T) {
	withFakeStore(t, &fakeStore{})

	t.Run("missing vault path", func(t *testing.T) {
		err := (&RefsCmd{Note: "router"}).Run(&Globals{Ctx: context.Background()})
		var usage *UsageError
		if !errors.As(err, &usage) {
			t.Errorf("expected UsageError, got %T: %v", err, err)
		}
	})
	t.Run("missing note", func(t *testing.T) {
		err := (&RefsCmd{}).Run(refsTestGlobals(t.TempDir()))
		var usage *UsageError
		if !errors.As(err, &usage) {
			t.Errorf("expected UsageError, got %T: %v", err, err)
		}
	})
}

func TestRefsNoteNotFoundPassthrough(t *testing.T) {
	fs := &fakeStore{noteMetaErr: errors.New("note not found: nope")}
	withFakeStore(t, fs)

	err := (&RefsCmd{Note: "nope"}).Run(refsTestGlobals(t.TempDir()))
	if err == nil || err.Error() != "note not found: nope" {
		t.Errorf("store error must pass through untouched, got %v", err)
	}
}

func TestPrintRefsFormattedToWriter(t *testing.T) {
	env := refsEnvelope{
		Command:  "refs",
		NoteSlug: "router",
		Title:    "Router",
		Total:    2,
		Refs: []refEntry{
			{Path: "/vault/Notes/cover.png", RelativePath: "Notes/cover.png", Kind: kindImage, Missing: false},
			{Path: "https://example.com", Kind: kindExternal, Missing: false},
		},
	}
	globals := &Globals{}

	t.Run("text", func(t *testing.T) {
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, env, globals); err != nil {
			t.Fatal(err)
		}
		want := "[image] /vault/Notes/cover.png\n[external-links] https://example.com\n"
		if sb.String() != want {
			t.Errorf("text = %q, want %q", sb.String(), want)
		}
	})

	t.Run("text marks missing", func(t *testing.T) {
		envMissing := env
		envMissing.Refs = append([]refEntry(nil), env.Refs...)
		envMissing.Refs[0].Missing = true
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, envMissing, globals); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "(missing)") {
			t.Errorf("missing marker absent: %q", sb.String())
		}
	})

	t.Run("json", func(t *testing.T) {
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, env, &Globals{Format: formatJSON}); err != nil {
			t.Fatal(err)
		}
		out := sb.String()
		for _, want := range []string{`"command": "refs"`, `"total": 2`, `"relative_path": "Notes/cover.png"`, `"missing": false`} {
			if !strings.Contains(out, want) {
				t.Errorf("json missing %s:\n%s", want, out)
			}
		}
	})

	t.Run("tsv", func(t *testing.T) {
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, env, &Globals{Format: formatTSV}); err != nil {
			t.Fatal(err)
		}
		want := "path\tkind\tmissing\trelative_path\n/vault/Notes/cover.png\timage\tfalse\tNotes/cover.png\nhttps://example.com\texternal-links\t\t\n"
		if sb.String() != want {
			t.Errorf("tsv = %q, want %q", sb.String(), want)
		}
	})

	t.Run("jsonpath", func(t *testing.T) {
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, env, &Globals{JSONPath: "$.refs[*].path"}); err != nil {
			t.Fatal(err)
		}
		want := "/vault/Notes/cover.png\nhttps://example.com\n"
		if sb.String() != want {
			t.Errorf("jsonpath = %q, want %q", sb.String(), want)
		}
	})

	t.Run("empty text", func(t *testing.T) {
		empty := refsEnvelope{Command: "refs", NoteSlug: "x", Refs: nil}
		var sb strings.Builder
		if err := printRefsFormattedToWriter(&sb, empty, globals); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(sb.String(), "No references found") {
			t.Errorf("empty text should print a notice, got %q", sb.String())
		}
	})
}
