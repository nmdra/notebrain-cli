package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsCmd = &cobra.Command{
	Use:    "gendocs",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		err := os.MkdirAll("docs/cli", 0755)
		if err != nil {
			log.Fatal(err)
		}
		err = doc.GenMarkdownTree(rootCmd, "docs/cli")
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}
