package store

import (
	"context"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/parser"
)

// TestPDFConnections_TwoStageIngest reproduces the exact user scenario:
//
//  1. First ingest: Only .md files. The note "00fleeting-notes/attention-is-all-you-need.md"
//     contains [[attention-is-all-you-need.pdf]]. Since the PDF isn't ingested yet,
//     the link's target_slug is resolved via parser.Slugify fallback.
//
//  2. Second ingest: PDFs are now enabled. The PDF "attention-is-all-you-need.pdf"
//     gets ingested. But the .md file is unchanged (same hash), so its links are NOT re-upserted.
//
//  3. Query: "connections" for the .md note should show the PDF as a connection,
//     but it doesn't because the stored target_slug from step 1 doesn't match
//     the PDF's actual note_slug.
func TestPDFConnections_TwoStageIngest(t *testing.T) {
	if testing.Short() {
		t.Skip("native Chroma integration requires a provisioned runtime")
	}
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	mdSlug := parser.Slugify("00fleeting-notes/attention-is-all-you-need.md")
	// What the wikilink [[attention-is-all-you-need.pdf]] resolves to via Slugify fallback
	// when the PDF doesn't exist yet:
	wikilinkFallbackSlug := parser.Slugify("attention-is-all-you-need.pdf")
	// What the PDF's actual slug is when it's ingested from the vault:
	pdfActualSlug := parser.Slugify("attention-is-all-you-need.pdf")

	t.Logf("md slug:               %s", mdSlug)
	t.Logf("wikilink fallback slug: %s", wikilinkFallbackSlug)
	t.Logf("pdf actual slug:        %s", pdfActualSlug)

	emb := make([]float32, 384)

	// ── Step 1: First ingest (no PDFs) ──
	// The .md note links to [[attention-is-all-you-need.pdf]].
	// Since the PDF isn't in the DB yet, resolveTargetSlug falls back to
	// parser.Slugify("attention-is-all-you-need.pdf") = "attention-is-all-you-need-pdf".
	err = st.BatchIngest(ctx, []BatchIngestData{
		{
			NoteSlug: mdSlug,
			ChunkRecords: []ChunkRecord{
				{
					ID:         mdSlug + ":0",
					NoteSlug:   mdSlug,
					ChunkIndex: 0,
					Title:      "Attention Is All You Need",
					FilePath:   "00fleeting-notes/attention-is-all-you-need.md",
					FileType:   "md",
					Embedding:  emb,
				},
			},
			Links: []string{"attention-is-all-you-need.pdf"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Step 1 (md ingest): %v", err)
	}

	// Verify the link was stored
	res, err := st.Connections(ctx, mdSlug, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("After step 1 (md only), connections: %d", len(res))
	for _, r := range res {
		t.Logf("  slug=%s phantom=%v", r.NoteSlug, r.IsPhantom)
	}
	// At this point the PDF link should exist but be phantom
	if len(res) != 1 {
		t.Fatalf("Expected 1 connection (phantom PDF), got %d", len(res))
	}
	if !res[0].IsPhantom {
		t.Errorf("Expected PDF connection to be phantom before PDF ingest")
	}

	// ── Step 2: Second ingest (with PDFs) ──
	// The PDF is now ingested. But the .md file is unchanged, so it's NOT in this batch.
	// Only the PDF data is in the batch.
	err = st.BatchIngest(ctx, []BatchIngestData{
		{
			NoteSlug: pdfActualSlug,
			ChunkRecords: []ChunkRecord{
				{
					ID:         pdfActualSlug + ":0",
					NoteSlug:   pdfActualSlug,
					ChunkIndex: 0,
					Title:      "attention-is-all-you-need",
					FilePath:   "attention-is-all-you-need.pdf",
					FileType:   "pdf",
					Embedding:  emb,
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Step 2 (pdf ingest): %v", err)
	}

	// ── Step 3: Query connections ──
	res, err = st.Connections(ctx, mdSlug, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("After step 2 (pdf added), connections: %d", len(res))
	for _, r := range res {
		t.Logf("  slug=%s title=%q phantom=%v", r.NoteSlug, r.Title, r.IsPhantom)
	}

	// The PDF should now be a real (non-phantom) connection
	if len(res) != 1 {
		t.Fatalf("Expected 1 connection, got %d", len(res))
	}
	if res[0].IsPhantom {
		t.Errorf("PDF should NOT be phantom after ingestion, but it is (slug=%s)", res[0].NoteSlug)
	}
}

// TestPDFConnections_TwoStageIngest_Subfolder tests when the PDF is in a different
// subfolder than the wikilink text suggests (Obsidian resolves by filename only).
func TestPDFConnections_TwoStageIngest_Subfolder(t *testing.T) {
	if testing.Short() {
		t.Skip("native Chroma integration requires a provisioned runtime")
	}
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	mdSlug := parser.Slugify("notes/my-note.md")
	// The wikilink says [[attention-is-all-you-need.pdf]] but the PDF is at assets/attention-is-all-you-need.pdf
	pdfActualSlug := parser.Slugify("assets/attention-is-all-you-need.pdf")

	t.Logf("md slug:         %s", mdSlug)
	t.Logf("pdf actual slug: %s", pdfActualSlug)

	emb := make([]float32, 384)

	// Step 1: Ingest .md without PDFs
	err = st.BatchIngest(ctx, []BatchIngestData{
		{
			NoteSlug: mdSlug,
			ChunkRecords: []ChunkRecord{
				{
					ID: mdSlug + ":0", NoteSlug: mdSlug, ChunkIndex: 0,
					Title: "My Note", FilePath: "notes/my-note.md", FileType: "md", Embedding: emb,
				},
			},
			Links: []string{"attention-is-all-you-need.pdf"},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Step 2: Ingest PDF (in different folder)
	err = st.BatchIngest(ctx, []BatchIngestData{
		{
			NoteSlug: pdfActualSlug,
			ChunkRecords: []ChunkRecord{
				{
					ID: pdfActualSlug + ":0", NoteSlug: pdfActualSlug, ChunkIndex: 0,
					Title: "attention-is-all-you-need", FilePath: "assets/attention-is-all-you-need.pdf",
					FileType: "pdf", Embedding: emb,
				},
			},
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Step 3: Query
	res, err := st.Connections(ctx, mdSlug, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Connections: %d", len(res))
	for _, r := range res {
		t.Logf("  slug=%s title=%q phantom=%v", r.NoteSlug, r.Title, r.IsPhantom)
	}

	if len(res) != 1 {
		t.Fatalf("Expected 1 connection, got %d", len(res))
	}
	if res[0].IsPhantom {
		t.Errorf("PDF should NOT be phantom (slug=%s), but the link target_slug was never reconciled", res[0].NoteSlug)
	}
	if res[0].NoteSlug != pdfActualSlug {
		t.Errorf("Expected connection to canonical slug %q, got %q", pdfActualSlug, res[0].NoteSlug)
	}
}
