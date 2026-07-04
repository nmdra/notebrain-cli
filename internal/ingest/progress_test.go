// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package ingest

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRunProgress(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)

	progressCh := make(chan ProgressUpdate, 10)
	progressCh <- ProgressUpdate{Done: 50, Current: "note50.md"}
	progressCh <- ProgressUpdate{Done: 100, Current: "note100.md", Final: true}
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
