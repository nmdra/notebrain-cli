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
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type ResetCmd struct {
}

func (c *ResetCmd) Run(globals *Globals) error {
	fmt.Println("WARNING: This will delete ALL indexed data (nb_chunks and nb_links).")
	fmt.Print("Type 'yes' to confirm: ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}

	if strings.TrimSpace(answer) != "yes" {
		fmt.Println("Reset cancelled.")
		return nil
	}

	chromaPath := globals.ChromaPath
	ctx := globals.Ctx
	st, err := store.Open(ctx, chromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if err := st.Reset(ctx); err != nil {
		return err
	}

	fmt.Println("Database reset successful. All collections dropped.")
	return nil
}
