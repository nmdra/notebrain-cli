package cmd

import (
	"testing"
)

func TestNormalizeTagInput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#kubernetes", "kubernetes"},
		{"kubernetes", "kubernetes"},
		{"  #Kubernetes  ", "kubernetes"},
		{"#kubernetes/cka", "kubernetes/cka"},
		{"", ""},
		{"#", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeTagInput(tt.input); got != tt.want {
				t.Errorf("normalizeTagInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
