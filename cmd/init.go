package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type InitCmd struct{}

// existingConfig mirrors the config keys the wizard can prefill.
type existingConfig struct {
	VaultPath string `toml:"vault-path"`
	WithPDF   bool   `toml:"with-pdf"`
}

func (c *InitCmd) Run(globals *Globals) error {
	initStyles()

	// One reader shared by every prompt: a fresh bufio.Reader per prompt
	// would over-read the pipe and discard buffered input, breaking piped
	// answers like `yes | notebrain init`.
	reader := bufio.NewReader(os.Stdin)

	fmt.Println(headerStyle.Render("NoteBrain CLI Initialization"))
	fmt.Println("This wizard will help you create a configuration file at ~/.notebrain/config/config.toml")
	fmt.Println()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	configDir := filepath.Join(home, ".notebrain", "config")
	configPath := filepath.Join(configDir, "config.toml")

	// Prefill from an existing config so re-running the wizard does not lose
	// the user's previous settings.
	var existing existingConfig
	if data, rerr := os.ReadFile(configPath); rerr == nil {
		_ = toml.Unmarshal(data, &existing)
	}

	if _, err := os.Stat(configPath); err == nil {
		printWarning("Config Exists", fmt.Sprintf("A configuration file already exists at %s", configPath))
		if !askYesNo(reader, "Do you want to overwrite it?", false) {
			fmt.Println("Initialization aborted.")
			return nil
		}
	}

	// Ask for vault path
	defaultVault := existing.VaultPath
	if defaultVault == "" {
		defaultVault = filepath.Join(home, "Documents", "Obsidian")
	}
	vaultPath := askString(reader, "Where is your Obsidian Vault located?", defaultVault)

	// Expand tilde if user typed it
	if strings.HasPrefix(vaultPath, "~/") {
		vaultPath = filepath.Join(home, vaultPath[2:])
	}

	// Warn about a vault path that does not exist, so the user can fix the
	// typo before the config is written.
	if info, serr := os.Stat(vaultPath); serr != nil || !info.IsDir() {
		printWarning("Vault Path", fmt.Sprintf("%q does not exist or is not a directory. Check the path.", vaultPath))
	}

	// Ask for PDF support
	enablePDF := askYesNo(reader, "Enable text extraction for PDF attachments?", existing.WithPDF)

	// Preview the changes before writing anything.
	fmt.Println()
	printWarning("Ready to write", fmt.Sprintf("vault-path = %q\nwith-pdf   = %t", vaultPath, enablePDF))
	if !askYesNo(reader, fmt.Sprintf("Write configuration to %s?", configPath), true) {
		fmt.Println("Initialization aborted.")
		return nil
	}

	// Prepare config file content
	configStr := string(globals.DefaultConfig)

	// Replace vault-path
	targetVaultLine := `# vault-path = "/home/user/Documents/Obsidian Vault"`
	newVaultLine := fmt.Sprintf(`vault-path = %q`, vaultPath)
	configStr = strings.Replace(configStr, targetVaultLine, newVaultLine, 1)

	// Replace PDF flag. The target line is the single with-pdf key in the
	// Ingestion Pipeline Settings section; config keys are flat, so this one
	// key governs both ingest and search.
	if enablePDF {
		configStr = strings.Replace(configStr, "# with-pdf = false", "with-pdf = true", 1)
	}

	// Write the config file
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(configPath, []byte(configStr), 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Println()
	printSuccess("Configuration Saved", configPath)
	fmt.Println(hintStyle.Render("You can now run 'notebrain ingest' to index your vault!"))

	return nil
}

func askString(reader *bufio.Reader, prompt, defaultValue string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultValue)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func askYesNo(reader *bufio.Reader, prompt string, defaultYes bool) bool {
	options := "[y/N]"
	if defaultYes {
		options = "[Y/n]"
	}

	fmt.Printf("%s %s: ", prompt, options)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input == "" {
		return defaultYes
	}
	return input == "y" || input == "yes"
}
