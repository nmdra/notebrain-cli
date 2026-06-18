package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestSearchCmd_MissingArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"search"})

	var out bytes.Buffer
	rootCmd.SetErr(&out)

	err := rootCmd.Execute()
	if err == nil {
		t.Fatalf("Expected an error when running search without args")
	}

	output := out.String()
	if !strings.Contains(output, "accepts 1 arg(s), received 0") {
		t.Errorf("Expected argument error, got: %s", output)
	}
}
