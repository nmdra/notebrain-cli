// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCmd_Run(t *testing.T) {
	var buf bytes.Buffer
	cmd := &VersionCmd{Writer: &buf}
	globals := &Globals{
		VersionString: "notebrain v1.2.0 (commit: abc1234, built: 2026-07-04T12:00:00Z)",
	}

	err := cmd.Run(globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := buf.String()
	want := "notebrain v1.2.0 (commit: abc1234, built: 2026-07-04T12:00:00Z)\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVersionCmd_Run_EmptyVersionString(t *testing.T) {
	var buf bytes.Buffer
	cmd := &VersionCmd{Writer: &buf}
	globals := &Globals{VersionString: ""}

	err := cmd.Run(globals)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(buf.String(), "\n") {
		t.Errorf("expected newline in output, got %q", buf.String())
	}
}
