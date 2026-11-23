package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nmdra/notebrain-cli/v2/internal/mcpbridge"
)

// MCPCmd groups the stdio MCP server command.
type MCPCmd struct {
	Serve MCPServeCmd `cmd:"" help:"Serve retrieval tools over stdio (one process per client)"`
}

// MCPServeCmd starts the local retrieval-only MCP server.
type MCPServeCmd struct {
	defaults              map[string]string
	version               string
	MaxConcurrentRequests int `name:"max-concurrent-requests" default:"4" help:"Maximum admitted tool calls (1..16); excess calls return busy"`
}

// Validate rejects unsupported startup concurrency limits as usage errors.
func (c *MCPServeCmd) Validate() error {
	if c.MaxConcurrentRequests < 1 || c.MaxConcurrentRequests > 16 {
		return fmt.Errorf("--max-concurrent-requests must be between 1 and 16")
	}
	return nil
}

// Run serves MCP messages on stdin/stdout without opening native storage.
func (c *MCPServeCmd) Run(g *Globals) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate NoteBrain executable: %w", err)
	}
	chroma, err := mcpAbsolutePath(g.ChromaPath)
	if err != nil {
		return fmt.Errorf("resolve Chroma path: %w", err)
	}
	vault, err := mcpAbsolutePath(g.VaultPath)
	if err != nil {
		return fmt.Errorf("resolve vault path: %w", err)
	}
	globalArgs := []string{
		"--chroma-path=" + chroma, "--vault-path=" + vault, "--vault-name=" + g.VaultName,
		"--skip-phantom=" + strconv.FormatBool(g.SkipPhantom), "--show-tags=" + strconv.FormatBool(g.ShowTags),
		"--show-file-path=" + strconv.FormatBool(g.ShowFilePath),
	}
	runner, err := mcpbridge.NewRunner(executable, globalArgs, c.defaults)
	if err != nil {
		return fmt.Errorf("prepare MCP process runner: %w", err)
	}
	server, err := mcpbridge.NewServer(c.version, c.MaxConcurrentRequests, c.defaults, runner)
	if err != nil {
		return errors.Join(fmt.Errorf("configure MCP server: %w", err), runner.Close())
	}
	if err := server.Run(g.Ctx); err != nil {
		return fmt.Errorf("serve MCP: %w", err)
	}
	return nil
}

// A missing vault is valid for discovery and metadata-only commands.
func mcpAbsolutePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if relative, ok := strings.CutPrefix(path, "~/"); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand home folder: %w", err)
		}
		path = filepath.Join(home, relative)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("make absolute path: %w", err)
	}
	return absolute, nil
}
