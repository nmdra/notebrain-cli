// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package cmd

import (
	"fmt"
	"io"
	"os"
)

// VersionCmd prints version information.
type VersionCmd struct {
	// Writer allows overriding destination for testing (defaults to os.Stdout)
	Writer io.Writer `kong:"-"`
}

// Run executes the version command.
func (c *VersionCmd) Run(globals *Globals) error {
	w := c.Writer
	if w == nil {
		w = os.Stdout
	}
	_, err := fmt.Fprintln(w, globals.VersionString)
	return err
}
