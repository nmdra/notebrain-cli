package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type GetCmd struct {
	Slug string `arg:"" help:"note slug, title, or file path (auto-resolved)" completion-predictor:"note-slug"`
	Meta bool   `group:"get" help:"show only the note header (title, path, tags, chunk count) without any text" default:"false"`
	Head int    `group:"get" help:"show only the first N chunks of text (0 = full note)" default:"0"`
}

func (c *GetCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := openStore(ctx, globals)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	var note *store.NoteContent
	switch {
	case c.Meta:
		note, err = st.GetNoteMeta(ctx, c.Slug)
	case c.Head > 0:
		note, err = st.GetNoteHead(ctx, c.Slug, c.Head)
	default:
		note, err = st.GetNote(ctx, c.Slug)
	}
	if err != nil {
		return err
	}

	if globals.JSONPath != "" {
		return printJSONPathResult(note, globals.JSONPath)
	}

	switch globals.Format {
	case formatJSON:
		env := struct {
			Command string             `json:"command"`
			Query   string             `json:"query"`
			Note    *store.NoteContent `json:"note"`
		}{
			Command: groupGet,
			Query:   c.Slug,
			Note:    note,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)

	case formatTSV:
		fmt.Println("note_slug\ttitle\tfile_path\ttags\tchunks\ttext")
		tagsStr := formatTags(note.Tags)
		fmt.Printf("%s\t%s\t%s\t%s\t%d\t%s\n",
			note.NoteSlug, note.Title, note.FilePath, tagsStr, note.Chunks, note.Text)
		return nil

	default: // "text"
		initStyles()
		fmt.Println(titleStyle.Render(note.Title))
		if note.FilePath != "" {
			fmt.Println(metaStyle.Render("Path:   " + note.FilePath))
		}
		if len(note.Tags) > 0 {
			fmt.Println(metaStyle.Render("Tags:   " + strings.Join(formatTagChips(note.Tags), " ")))
		}
		fmt.Println(metaStyle.Render(fmt.Sprintf("Chunks: %d", note.Chunks)))
		if note.Text != "" {
			fmt.Println("\n" + note.Text + "\n")
		} else {
			fmt.Println()
		}
		return nil
	}
}
