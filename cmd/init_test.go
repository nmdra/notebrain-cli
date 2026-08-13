package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initTestConfig is a minimal stand-in for config.example.toml containing the
// marker lines the init wizard rewrites.
const initTestConfig = `# vault-path = "/home/user/Documents/Obsidian Vault"
# with-pdf = false
`

func TestInitCmdWritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	withStdin(t, "\ny\nyes\n")

	globals := &Globals{
		Ctx:           context.Background(),
		DefaultConfig: []byte(initTestConfig),
	}
	if err := (&InitCmd{}).Run(globals); err != nil {
		t.Fatalf("Run: %v", err)
	}

	configPath := filepath.Join(home, ".notebrain", "config", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	s := string(data)
	wantVault := fmt.Sprintf("vault-path = %q", filepath.Join(home, "Documents", "Obsidian"))
	if !strings.Contains(s, wantVault) {
		t.Errorf("config missing %q, got:\n%s", wantVault, s)
	}
	if !strings.Contains(s, "with-pdf = true") {
		t.Errorf("config missing with-pdf = true, got:\n%s", s)
	}
}

func TestInitCmdDeclineSkipsWrite(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	withStdin(t, "\nn\nn\n")

	globals := &Globals{
		Ctx:           context.Background(),
		DefaultConfig: []byte(initTestConfig),
	}
	if err := (&InitCmd{}).Run(globals); err != nil {
		t.Fatalf("Run: %v", err)
	}

	configPath := filepath.Join(home, ".notebrain", "config", "config.toml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("config %s should not exist after declining the write", configPath)
	}
}
