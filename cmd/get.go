package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/nmdra/notebrain-cli/v2/internal/store"
)

type GetCmd struct {
	Slug string `arg:"" help:"Note slug or file path to retrieve"`
}

func (c *GetCmd) Run(globals *Globals) error {
	ctx := globals.Ctx
	st, err := store.Open(ctx, globals.ChromaPath)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	note, err := st.GetNote(ctx, c.Slug)
	if err != nil {
		return err
	}

	if globals.JSONPath != "" {
		return printJSONPathResult(note, globals.JSONPath)
	}

	switch globals.Format {
	case "json", "ndjson":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(note)

	case "tsv":
		fmt.Println("note_slug\ttitle\tfile_path\ttags\tchunks\ttext")
		tagsStr := strings.Join(note.Tags, ",")
		fmt.Printf("%s\t%s\t%s\t%s\t%d\t%s\n",
			note.NoteSlug, note.Title, note.FilePath, tagsStr, note.Chunks, note.Text)
		return nil

	default: // "text"
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
		metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

		fmt.Println(titleStyle.Render(note.Title))
		if note.FilePath != "" {
			fmt.Println(metaStyle.Render("Path:   " + note.FilePath))
		}
		if len(note.Tags) > 0 {
			chips := make([]string, 0, len(note.Tags))
			for _, t := range note.Tags {
				chips = append(chips, "#"+t)
			}
			fmt.Println(metaStyle.Render("Tags:   " + strings.Join(chips, " ")))
		}
		fmt.Println(metaStyle.Render(fmt.Sprintf("Chunks: %d", note.Chunks)))
		fmt.Println("\n" + note.Text + "\n")
		return nil
	}
}
