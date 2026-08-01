// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

// failOnTextEmbedder fails Embed for any text containing the marker, so a
// file can be forced to fail deterministically.
type failOnTextEmbedder struct{ marker string }

func (m *failOnTextEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.Contains(text, m.marker) {
		return nil, fmt.Errorf("embedding refused for %q", m.marker)
	}
	return []float32{1.0, 0.0, 0.0}, nil
}

func (m *failOnTextEmbedder) Model() string { return "mock" }

func TestRunProgress(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)

	progressCh := make(chan ProgressUpdate, 10)
	progressCh <- ProgressUpdate{Done: 50, Current: "note50.md"}
	progressCh <- ProgressUpdate{Done: 100, Current: "note100.md"}
	close(progressCh)

	RunProgress(100, progressCh)

	out := buf.String()
	if !strings.Contains(out, "ingestion progress") {
		t.Errorf("expected log output to contain 'ingestion progress', got:\n%s", out)
	}
	if !strings.Contains(out, "ingestion completed") {
		t.Errorf("expected log output to contain 'ingestion completed', got:\n%s", out)
	}
}

// TestRunProgressCountsFailedFiles verifies that failed files still advance
// the progress counter, so percent reaches 100 even when some files error out.
func TestRunProgressCountsFailedFiles(t *testing.T) {
	ctx := context.Background()
	dbDir := t.TempDir()
	st, err := store.Open(ctx, dbDir)
	if err != nil {
		t.Fatalf("Failed to open store: %v", err)
	}
	defer func() { _ = st.Close() }()

	vaultDir := t.TempDir()
	writeTestFile(t, filepath.Join(vaultDir, "good.md"), "This is a note with enough words to embed.")
	writeTestFile(t, filepath.Join(vaultDir, "bad.md"), "This file always fails to embed.")

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)

	p := NewPipeline(st, &failOnTextEmbedder{marker: "fails"}, 2)
	p.MinChunkWords = 0

	err = p.Run(ctx, vaultDir, "")
	if err == nil {
		t.Fatal("expected Run to report the failed file")
	}
	if failed := p.FailedFiles(); len(failed) != 1 {
		t.Fatalf("expected 1 failed file, got %d", len(failed))
	}

	type progressLog struct {
		Msg       string `json:"msg"`
		Processed int    `json:"processed"`
		Total     int    `json:"total"`
	}
	var last progressLog
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var entry progressLog
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Msg == "ingestion progress" {
			last = entry
		}
	}
	if last.Total != 2 || last.Processed != 2 {
		t.Errorf("progress did not count the failed file: got processed=%d total=%d, want 2/2", last.Processed, last.Total)
	}
}
