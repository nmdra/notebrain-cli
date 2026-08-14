/*
Copyright © 2026 nmdra

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type StatsCmd struct {
}

func (c *StatsCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	stats, err := st.Stats(ctx)
	if err != nil {
		return err
	}

	env := struct {
		Command string `json:"command"`
		*store.Stats
	}{
		Command: groupStats,
		Stats:   stats,
	}

	if handled, err := printEnvelopeJSON(os.Stdout, env, globals); handled {
		return err
	}

	if globals.Format == formatTSV {
		fmt.Println("notes\tchunks\tlinks")
		fmt.Printf("%d\t%d\t%d\n", stats.Notes, stats.Chunks, stats.Links)
		return nil
	}

	initStyles()
	rows := fmt.Sprintf(
		"%s  %d\n%s  %d\n%s  %d",
		labelStyle.Render("Notes "), stats.Notes,
		labelStyle.Render("Chunks"), stats.Chunks,
		labelStyle.Render("Links "), stats.Links,
	)

	fmt.Println()
	fmt.Println(boxStyle.Render(rows))
	fmt.Println()
	return nil
}
