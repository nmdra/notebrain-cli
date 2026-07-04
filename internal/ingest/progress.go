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
	Final   bool
}

// RunProgress logs ingestion progress via structured slog events.
func RunProgress(totalFiles int, progressCh <-chan ProgressUpdate) error {
	step := totalFiles / 5
	if step < 10 {
		step = 10
	}
	if step > totalFiles && totalFiles > 0 {
		step = totalFiles
	}
	if step == 0 {
		step = 1
	}
	lastLogged := -step - 1
	start := time.Now()

	for u := range progressCh {
		if u.Done >= lastLogged+step || u.Done == totalFiles || u.Final {
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
			lastLogged = u.Done
		}
		if u.Final {
			break
		}
	}
	slog.Info("ingestion completed",
		"total_files", totalFiles,
		"duration_ms", time.Since(start).Milliseconds())
	return nil
}
