package cmd

import (
	"fmt"
	"os"
	"sort"
)

// SuggestNotesCmd lists all indexed note slugs, one per line.
// It is hidden from help and shell completion; the note-slug completion
// predictor shells out to it to fetch dynamic slug candidates.
type SuggestNotesCmd struct{}

func (c *SuggestNotesCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	metas, err := st.GetNoteMetadata(ctx)
	if err != nil {
		return err
	}

	slugs := make([]string, 0, len(metas))
	for slug := range metas {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		fmt.Fprintln(os.Stdout, slug)
	}
	return nil
}
