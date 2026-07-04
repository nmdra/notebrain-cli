// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package tui

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRunSpinner_Headless(t *testing.T) {
	t.Setenv("TERM", "dumb")
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	oldLogger := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldLogger)

	done := make(chan struct{})
	go func() {
		close(done)
	}()

	err := RunSpinner("testing spinner", done)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "testing spinner") {
		t.Errorf("expected log output to contain 'testing spinner', got: %s", buf.String())
	}
}
