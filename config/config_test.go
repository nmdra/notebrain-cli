// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

package config

import (
	"strings"
	"testing"
)

func TestDefault_NonNil(t *testing.T) {
	cfg := Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}
}

func TestDefault_Fields(t *testing.T) {
	tests := []struct {
		name string
		got  func(*Config) any
		want any
	}{
		{
			name: "ChromaPath ends with .notebrain/chroma",
			got: func(c *Config) any {
				return strings.HasSuffix(c.ChromaPath, ".notebrain/chroma")
			},
			want: true,
		},
		{
			name: "ChromaMode is persistent",
			got:  func(c *Config) any { return c.ChromaMode },
			want: "persistent",
		},
		{
			name: "ChromaURL",
			got:  func(c *Config) any { return c.ChromaURL },
			want: "http://localhost:8000",
		},
		{
			name: "Embedder is minilm",
			got:  func(c *Config) any { return c.Embedder },
			want: "minilm",
		},
		{
			name: "OllamaURL",
			got:  func(c *Config) any { return c.OllamaURL },
			want: "http://localhost:11434",
		},
		{
			name: "OllamaModel",
			got:  func(c *Config) any { return c.OllamaModel },
			want: "nomic-embed-text",
		},
		{
			name: "ChunkSize is 512",
			got:  func(c *Config) any { return c.ChunkSize },
			want: 512,
		},
		{
			name: "ChunkOverlap is 64",
			got:  func(c *Config) any { return c.ChunkOverlap },
			want: 64,
		},
		{
			name: "Limit is 10",
			got:  func(c *Config) any { return c.Limit },
			want: 10,
		},
		{
			name: "Verbose is false",
			got:  func(c *Config) any { return c.Verbose },
			want: false,
		},
		{
			name: "MinChunkWords is 0",
			got:  func(c *Config) any { return c.MinChunkWords },
			want: 0,
		},
	}

	cfg := Default()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.got(cfg)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
