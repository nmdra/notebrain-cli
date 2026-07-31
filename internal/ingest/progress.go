// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package ingest

import (
	"log/slog"
	"math"
	"time"
)

// ProgressUpdate represents a status update during file processing.
type ProgressUpdate struct {
	Done    int
	Total   int
	Current string
}

// RunProgress logs ingestion progress via structured slog events for every file.
func RunProgress(totalFiles int, progressCh <-chan ProgressUpdate) {
	start := time.Now()

	for u := range progressCh {
		percent := 0.0
		if totalFiles > 0 {
			percent = math.Round(float64(u.Done)/float64(totalFiles)*10000) / 100
		}
		slog.Info("ingestion progress",
			"processed", u.Done,
			"total", totalFiles,
			"percent", percent,
			"current", u.Current,
			"elapsed_ms", time.Since(start).Milliseconds())
	}
	slog.Info("ingestion completed",
		"total_files", totalFiles,
		"duration_ms", time.Since(start).Milliseconds())
}
