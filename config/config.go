// Copyright © 2026 nmdra. All rights reserved.
// Use of this source code is governed by the MIT license
// that can be found in the LICENSE file.

// Package config holds all configuration for notebrain-cli.
package config

import (
	"os"
	"path/filepath"
)

// Config holds all configuration for notebrain-cli.
type Config struct {
	// ChromaPath is the local directory where ChromaDB stores its data.
	ChromaPath string
	// ChromaMode selects persistent (embedded) or http (standalone server).
	ChromaMode string // "persistent" | "http"
	// ChromaURL is used when ChromaMode == "http".
	ChromaURL string
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
	Verbose        bool
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ChromaPath:     filepath.Join(home, ".notebrain", "chroma"),
		ChromaMode:     "persistent",
		ChromaURL:      "http://localhost:8000",
		Embedder:       "minilm",
		OllamaURL:      "http://localhost:11434",
		OllamaModel:    "nomic-embed-text",
		ChunkSize:      800, // runes; ~178 tokens — leaves ~78-token headroom for title/heading prefix
		ChunkOverlap:   100, // runes; ~1-2 sentences of overlap for sub-chunk splits
		Limit:          10,
		MinChunkWords:  10,  // rejects heading-only and code-placeholder fragments
		MaxEmbedTokens: 256, // matches MiniLM sequence length
	}
}
