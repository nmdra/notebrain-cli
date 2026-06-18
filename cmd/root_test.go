package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {
	// Set the args to empty to just trigger root command help or execution
	rootCmd.SetArgs([]string{"--help"})

	// Capture output
	var out bytes.Buffer
	rootCmd.SetOut(&out)

	err := rootCmd.Execute()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "NoteBrain indexes an Obsidian vault") {
		t.Errorf("Expected root command help to contain 'NoteBrain indexes an Obsidian vault', got: %s", output)
	}
}
