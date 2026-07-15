// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package config holds all configuration for notebrain-cli.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Config holds all configuration for notebrain-cli.
type Config struct {
	// ChromaPath is the local directory where ChromaDB stores its data.
	ChromaPath string
	// VaultName for Obsidian CLI targeting.
	VaultName string
	// Embedder type: "minilm" or "ollama"
	Embedder    string
	OllamaURL   string
	OllamaModel string
	// ChunkSize is the maximum number of runes per chunk passed to the parser.
	// MiniLM-L6-v2 is optimal at 128-256 tokens (~600-800 runes for English prose).
	ChunkSize int
	// ChunkOverlap is the number of runes repeated at the start of each sub-chunk
	// when a section is split. Provides sentence-level continuity across boundaries.
	ChunkOverlap int
	Limit        int
	// MinChunkWords filters out chunks with fewer words than this threshold before
	// embedding. Eliminates heading-only, code-placeholder, and link-only fragments.
	MinChunkWords int
	// MaxEmbedTokens is the maximum sequence length for embed text (model token budget).
	MaxEmbedTokens int
	// TopKPerNote limits the number of chunks returned per note in semantic search.
	TopKPerNote int
	Verbose     bool
	// LogFormat controls log output format: "auto", "json", or "text".
	LogFormat string
	// LogLevel controls minimum log severity: "info", "debug", "warn", or "error".
	LogLevel        string
	SkipAttachments bool
	SkipPhantom     bool
	HideTags        bool
	Compact         bool
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return &Config{
		ChromaPath:      filepath.Join(home, ".notebrain", "chroma"),
		Embedder:        "minilm",
		OllamaURL:       "http://localhost:11434",
		OllamaModel:     "nomic-embed-text",
		ChunkSize:       800, // runes; ~178 tokens — leaves ~78-token headroom for title/heading prefix
		ChunkOverlap:    100, // runes; ~1-2 sentences of overlap for sub-chunk splits
		Limit:           10,
		MinChunkWords:   10,  // rejects heading-only and code-placeholder fragments
		MaxEmbedTokens:  256, // matches MiniLM sequence length
		TopKPerNote:     3,   // top 3 most relevant chunks per note
		LogFormat:       "auto",
		LogLevel:        "info",
		SkipAttachments: true,
		SkipPhantom:     true,
		HideTags:        true,
		Compact:         false,
	}
}

// Validate checks configuration settings for invalid or inconsistent values.
func (c *Config) Validate() error {
	if c.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive, got %d", c.ChunkSize)
	}
	if c.ChunkOverlap < 0 {
		return fmt.Errorf("chunk overlap cannot be negative, got %d", c.ChunkOverlap)
	}
	if c.ChunkOverlap >= c.ChunkSize {
		return fmt.Errorf("chunk overlap (%d) must be less than chunk size (%d)", c.ChunkOverlap, c.ChunkSize)
	}
	if c.Limit <= 0 {
		return fmt.Errorf("limit must be positive, got %d", c.Limit)
	}
	if c.TopKPerNote <= 0 {
		return fmt.Errorf("top-k per note must be positive, got %d", c.TopKPerNote)
	}
	return nil
}
