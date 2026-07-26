package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type InitCmd struct{}

func (c *InitCmd) Run(globals *Globals) error {
	initStyles()

	fmt.Println(headerStyle.Render("NoteBrain CLI Initialization"))
	fmt.Println("This wizard will help you create a configuration file at ~/.notebrain/config/config.toml")
	fmt.Println()

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	configDir := filepath.Join(home, ".notebrain", "config")
	configPath := filepath.Join(configDir, "config.toml")

	if _, err := os.Stat(configPath); err == nil {
		printWarning("Config Exists", fmt.Sprintf("A configuration file already exists at %s", configPath))
		if !askYesNo("Do you want to overwrite it?", false) {
			fmt.Println("Initialization aborted.")
			return nil
		}
	}

	// Ask for vault path
	defaultVault := filepath.Join(home, "Documents", "Obsidian")
	vaultPath := askString("Where is your Obsidian Vault located?", defaultVault)

	// Expand tilde if user typed it
	if strings.HasPrefix(vaultPath, "~/") {
		vaultPath = filepath.Join(home, vaultPath[2:])
	}

	// Ask for PDF support
	enablePDF := askYesNo("Enable text extraction for PDF attachments?", false)
	enableOCR := false
	if enablePDF {
		enableOCR = askYesNo("Enable OCR fallback for scanned PDFs? (requires Tesseract to be installed)", false)
	}

	// Prepare config file content
	configStr := string(globals.DefaultConfig)

	// Replace vault-path
	targetVaultLine := `# vault-path = "/home/user/Documents/Obsidian Vault"`
	newVaultLine := fmt.Sprintf(`vault-path = %q`, vaultPath)
	configStr = strings.Replace(configStr, targetVaultLine, newVaultLine, 1)

	// Replace PDF and OCR flags
	if enablePDF {
		configStr = strings.Replace(configStr, "enable-pdf = false", "enable-pdf = true", 1)
	}
	if enableOCR {
		configStr = strings.Replace(configStr, "enable-ocr = false", "enable-ocr = true", 1)
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

func askString(prompt, defaultValue string) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s [%s]: ", prompt, defaultValue)

	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	if input == "" {
		return defaultValue
	}
	return input
}

func askYesNo(prompt string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
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
