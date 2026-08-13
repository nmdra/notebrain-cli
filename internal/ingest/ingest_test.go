package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0, 0.0, 0.0}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1.0, 0.0, 0.0}
	}
	return out, nil
}

func (m *mockEmbedder) Close() error { return nil }

func (m *mockEmbedder) Model() string { return "mock" }

// recordingEmbedder records every text sent to Embed so tests can inspect
// exactly what the embedding model received. It is safe for concurrent use:
// workers call Embed from multiple goroutines.
type recordingEmbedder struct {
	mu    sync.Mutex
	texts []string
}

func (m *recordingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return []float32{1.0, 0.0, 0.0}, nil
}

func (m *recordingEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, texts...)
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1.0, 0.0, 0.0}
	}
	return out, nil
}

func (m *recordingEmbedder) Close() error { return nil }

func (m *recordingEmbedder) Model() string { return "mock" }

func TestPipelineRun(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// Create some markdown files
	files := map[string]string{
		"note1.md":          "This is a note with a [[note2]] link and a #tag.",
		"note2.md":          "Another note. [[note1|backlink]]",
		"ignore.txt":        "Should be ignored",
		".hidden/hidden.md": "Should be ignored",
		"dir/nested.md":     "A nested markdown file",
	}

	for name, content := range files {
		path := filepath.Join(vaultDir, name)
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, []byte(content), 0644)
	}

	p := NewPipeline(st, &mockEmbedder{}, 2)
	p.MinChunkWords = 0

	err = p.Run(ctx, vaultDir, "")
	if err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats.Chunks != 3 {
		t.Errorf("Expected 3 chunks (note1, note2, nested), got %d", stats.Chunks)
	}
	if stats.Links != 2 {
		t.Errorf("Expected 2 links, got %d", stats.Links)
	}
}

func TestPipelineSyncDeleted(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// 1. Write note1.md and note2.md
	n1Path := filepath.Join(vaultDir, "note1.md")
	n2Path := filepath.Join(vaultDir, "note2.md")
	_ = os.WriteFile(n1Path, []byte("Note one [[note2]]"), 0644)
	_ = os.WriteFile(n2Path, []byte("Note two [[note1]]"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.MinChunkWords = 0

	// Ingest initially
	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("First run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats.Chunks != 2 || stats.Links != 2 {
		t.Fatalf("Expected 2 chunks, 2 links initially, got %v", stats)
	}

	// 2. Delete note2.md on disk
	if err := os.Remove(n2Path); err != nil {
		t.Fatalf("Failed to delete file: %v", err)
	}

	// Ingest again
	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Second run failed: %v", err)
	}

	// 3. Verify that note2 has been cleaned up (only 1 chunk and 1 link for note1 remain)
	stats, _ = st.Stats(ctx)
	if stats.Chunks != 1 {
		t.Errorf("Expected 1 chunk remaining after sync, got %d", stats.Chunks)
	}
	if stats.Links != 1 {
		t.Errorf("Expected 1 link remaining after sync, got %d", stats.Links)
	}
}

// TestPipeline_PreservesIndexWhenAllChunksFiltered verifies that raising
// --min-chunk-words does not delete previously indexed notes whose chunks
// now fall below the threshold.
func TestPipeline_PreservesIndexWhenAllChunksFiltered(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(vaultDir, "note1.md"), []byte("short note"), 0644)
	_ = os.WriteFile(filepath.Join(vaultDir, "note2.md"), []byte("this is a longer note containing several words"), 0644)

	run := func(minWords int) {
		p := NewPipeline(st, &mockEmbedder{}, 1)
		p.MinChunkWords = minWords
		if err := p.Run(ctx, vaultDir, ""); err != nil {
			t.Fatalf("Pipeline.Run failed: %v", err)
		}
	}

	run(0) // index everything
	stats, _ := st.Stats(ctx)
	if stats.Chunks != 2 {
		t.Fatalf("Expected 2 chunks after initial ingest, got %d", stats.Chunks)
	}

	run(50) // all chunks of note1 now filtered
	stats, _ = st.Stats(ctx)
	if stats.Chunks != 2 {
		t.Errorf("Expected previously indexed notes to be preserved, got %d chunks", stats.Chunks)
	}
}

func TestPipelineMinChunkWords(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()

	// note1 has 2 words (should be skipped if MinChunkWords = 5)
	// note2 has 8 words (should be kept if MinChunkWords = 5)
	_ = os.WriteFile(filepath.Join(vaultDir, "note1.md"), []byte("short note"), 0644)
	_ = os.WriteFile(filepath.Join(vaultDir, "note2.md"), []byte("this is a longer note containing several words"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.MinChunkWords = 0
	p.MinChunkWords = 5

	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats.Chunks != 1 {
		t.Errorf("Expected 1 chunk (only note2), got %d", stats.Chunks)
	}
}

func TestPipelineRespectExclude(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	obsidianDir := filepath.Join(vaultDir, ".obsidian")
	_ = os.MkdirAll(obsidianDir, 0755)
	appJSON := []byte(`{"userIgnoreFilters": ["Archive"], "attachmentFolderPath": "99.Storage-Shed/Attachments"}`)
	_ = os.WriteFile(filepath.Join(obsidianDir, "app.json"), appJSON, 0644)

	_ = os.WriteFile(filepath.Join(vaultDir, "active.md"), []byte("Active note content"), 0644)
	_ = os.MkdirAll(filepath.Join(vaultDir, "Archive"), 0755)
	_ = os.WriteFile(filepath.Join(vaultDir, "Archive", "old.md"), []byte("Old note content"), 0644)
	_ = os.MkdirAll(filepath.Join(vaultDir, "99.Storage-Shed", "Attachments"), 0755)
	_ = os.WriteFile(filepath.Join(vaultDir, "99.Storage-Shed", "Attachments", "attachment.md"), []byte("Attachment note"), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.MinChunkWords = 0
	p.RespectExclude = true

	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats.Chunks != 1 {
		t.Errorf("Expected 1 chunk (only active.md), got %d", stats.Chunks)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{"empty", ""},
		{"short", "hello world"},
		{"medium", strings.Repeat("word ", 50)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got < 0 {
				t.Errorf("estimateTokens(%q) = %d, want >= 0", tt.text, got)
			}
			if tt.text != "" && got < 1 {
				t.Errorf("estimateTokens(%q) = %d, want >= 1 for non-empty string", tt.text, got)
			}
		})
	}
}

func TestBuildEmbedText_TruncationGuard(t *testing.T) {
	longTitle := strings.Repeat("Architecture ", 20) // ~260 chars
	longHeading := strings.Repeat("SubSection > ", 15)
	tags := []string{"tag1", "tag2", "tag3", "tag4", "tag5"}
	body := "This is the chunk body that must be preserved."

	tests := []struct {
		name      string
		title     string
		heading   string
		tags      []string
		body      string
		maxTokens int
		wantTitle bool
		wantTags  bool
	}{
		{"normal fit", "My Note", "Section A", tags, body, 256, true, true},
		{"long title drops tags", longTitle, "Sec", tags, body, 80, true, false},
		{"very long prefix drops all", longTitle, longHeading, tags, body, 60, false, false},
		{"empty prefix", "", "", nil, body, 256, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildEmbedText(tt.title, tt.heading, tt.tags, tt.body, tt.maxTokens)
			if !strings.Contains(result, tt.body) {
				t.Errorf("body text missing from embed text: got %q", result)
			}
			if tt.wantTags && !strings.Contains(result, "[tags:") {
				t.Errorf("expected tags in embed text: got %q", result)
			}
			if !tt.wantTags && strings.Contains(result, "[tags:") {
				t.Errorf("expected tags to be trimmed: got %q", result)
			}
		})
	}
}

func TestPipeline_CodeOnlyNoteIngest(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	codeNote := "---\ntitle: Code Snippet\n---\n# Helper Function\n\n```go\nfunc add(a, b int) int {\n    return a + b\n}\n```\n"
	_ = os.WriteFile(filepath.Join(vaultDir, "code.md"), []byte(codeNote), 0644)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.MinChunkWords = 0
	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	stats, _ := st.Stats(ctx)
	if stats.Chunks != 1 {
		t.Errorf("Expected 1 chunk for code-only note, got %d", stats.Chunks)
	}
}

func TestPipeline_PDFFallbackPreservesPDFs(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(vaultDir, "doc.md"), []byte("MD Note"), 0644)

	// Seed store with a PDF note
	_ = st.BatchIngest(ctx, []store.BatchIngestData{
		{
			NoteSlug: "pdf-doc",
			ChunkRecords: []store.ChunkRecord{
				{
					ID:          "pdf-doc:0",
					NoteSlug:    "pdf-doc",
					Title:       "PDF Document",
					FilePath:    "PDF Document.pdf",
					ChunkIndex:  0,
					ContentHash: "abcdef",
					FileType:    store.FileTypePDF,
					Embedding:   []float32{1.0, 0.0, 0.0},
				},
			},
		},
	}, nil)

	p := NewPipeline(st, &mockEmbedder{}, 1)
	p.EnablePDF = true
	p.LLMModel = "" // Trigger fallback
	p.MinChunkWords = 0

	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	metas, err := st.GetNoteMetadata(ctx)
	if err != nil {
		t.Fatalf("Failed to get note metadata: %v", err)
	}

	if _, ok := metas["pdf-doc"]; !ok {
		t.Errorf("Expected PDF note to be preserved during fallback, but it was deleted")
	}
	if _, ok := metas["doc"]; !ok {
		t.Errorf("Expected MD note to be ingested")
	}
}

func BenchmarkEstimateTokens(b *testing.B) {
	text := strings.Repeat("This is a test sentence for token estimation in NoteBrain. ", 50)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = estimateTokens(text)
	}
}

func BenchmarkBuildEmbedText(b *testing.B) {
	title := "System Architecture and Data Flow"
	heading := "Internal Components > Ingestion Pipeline"
	tags := []string{"architecture", "golang", "chromadb", "embeddings"}
	body := strings.Repeat("The ingestion pipeline tokenizes markdown notes and stores them into ChromaDB vectors. ", 10)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		_ = buildEmbedText(title, heading, tags, body, 256)
	}
}

func tailRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}

// TestPipeline_StoredTextDedup_EmbedKeepsOverlap verifies the core fix:
// stored/displayed chunk text is overlap-free (each source sentence appears
// exactly once when the note is joined), while the embedding still receives
// the overlapping boundary tail of the previous chunk.
func TestPipeline_StoredTextDedup_EmbedKeepsOverlap(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	sentences := make([]string, 0, 100)
	for i := range 100 {
		sentences = append(sentences, fmt.Sprintf("Sentence %03d is a distinct chunk of prose. ", i))
	}
	body := strings.Join(sentences, " ")
	if err := os.WriteFile(filepath.Join(vaultDir, "long.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	rec := &recordingEmbedder{}
	p := NewPipeline(st, rec, 2)
	p.MinChunkWords = 0
	p.ChunkSize = 100
	p.ChunkOverlap = 20

	if err := p.Run(ctx, vaultDir, ""); err != nil {
		t.Fatalf("Pipeline.Run failed: %v", err)
	}

	note, err := st.GetNote(ctx, "long")
	if err != nil {
		t.Fatalf("GetNote failed: %v", err)
	}
	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if n := strings.Count(note.Text, s); n != 1 {
			t.Errorf("sentence %q appears %d times in stored text; want exactly 1", s, n)
		}
	}

	if len(rec.texts) != note.Chunks {
		t.Fatalf("embed calls %d != chunks %d", len(rec.texts), note.Chunks)
	}
	for i := 1; i < len(rec.texts); i++ {
		tail := tailRunes(rec.texts[i-1], 12)
		if tail == "" {
			continue
		}
		if !strings.Contains(rec.texts[i], tail) {
			t.Errorf("embed input %d is missing the boundary tail of embed input %d (%q)", i, i-1, tail)
		}
	}
}

func TestFileHashIncludesChunkParams(t *testing.T) {
	content := []byte("same content")
	const model = "mock"
	first := fileHash(content, 800, 100, model)
	second := fileHash(content, 800, 100, model)
	if first != second {
		t.Error("hash should be stable for identical content and params")
	}
	if fileHash(content, 800, 100, model) == fileHash(content, 400, 100, model) {
		t.Error("hash must change when chunk-size changes")
	}
	if fileHash(content, 800, 100, model) == fileHash(content, 800, 50, model) {
		t.Error("hash must change when chunk-overlap changes")
	}
	if fileHash(content, 800, 100, model) == fileHash([]byte("other content"), 800, 100, model) {
		t.Error("hash must change when content changes")
	}
	if fileHash(content, 800, 100, model) == fileHash(content, 800, 100, "other-model") {
		t.Error("hash must change when the embedding model changes")
	}
}

// TestPipeline_ReingestsWhenChunkParamsChange verifies a hash change caused by
// chunk parameters forces re-ingestion of an otherwise unchanged file.
func TestPipeline_ReingestsWhenChunkParamsChange(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	body := strings.Repeat("Long paragraph content with enough words that the section will definitely exceed the configured chunk size limit and split into multiple chunks. ", 30)
	if err := os.WriteFile(filepath.Join(vaultDir, "long.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	p := NewPipeline(st, &mockEmbedder{}, 2)
	p.MinChunkWords = 0

	run := func() {
		if err := p.Run(ctx, vaultDir, ""); err != nil {
			t.Fatalf("Pipeline.Run failed: %v", err)
		}
	}

	run() // initial ingest
	meta1, err := st.GetNoteMetadata(ctx)
	if err != nil {
		t.Fatalf("GetNoteMetadata failed: %v", err)
	}
	hash1 := meta1["long"].Hash

	run() // unchanged -> skip
	meta2, err := st.GetNoteMetadata(ctx)
	if err != nil {
		t.Fatalf("GetNoteMetadata failed: %v", err)
	}
	if meta2["long"].Hash != hash1 {
		t.Error("expected no re-ingest when content and chunk params are unchanged")
	}

	p.ChunkSize = 400
	p.ChunkOverlap = 50
	run() // chunk params changed -> re-ingest
	meta3, err := st.GetNoteMetadata(ctx)
	if err != nil {
		t.Fatalf("GetNoteMetadata failed: %v", err)
	}
	if meta3["long"].Hash == hash1 {
		t.Error("expected re-ingest when chunk params change")
	}
}
