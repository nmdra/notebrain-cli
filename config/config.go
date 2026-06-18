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
	ChunkSize   int
	ChunkOverlap int
	Limit       int
	Verbose     bool
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		ChromaPath:   filepath.Join(home, ".notebrain", "chroma"),
		ChromaMode:   "persistent",
		ChromaURL:    "http://localhost:8000",
		Embedder:     "minilm",
		OllamaURL:    "http://localhost:11434",
		OllamaModel:  "nomic-embed-text",
		ChunkSize:    512,
		ChunkOverlap: 64,
		Limit:        10,
	}
}
