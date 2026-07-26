package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"

	"github.com/nmdra/notebrain-cli/v2/internal/pdfextract"
)

type DoctorCmd struct{}

func (c *DoctorCmd) Run(globals *Globals) error {
	initStyles()

	fmt.Println(headerStyle.Render("NoteBrain CLI Diagnostics"))

	// 1. Vault Path
	vaultPath := globals.VaultPath
	if vaultPath == "" {
		printWarning("Vault Path", "Not configured in config.toml or flags.")
	} else {
		info, err := os.Stat(vaultPath)
		switch {
		case err != nil:
			printError("Vault Path", fmt.Sprintf("Cannot access %q: %v", vaultPath, err))
		case !info.IsDir():
			printError("Vault Path", fmt.Sprintf("%q is not a directory.", vaultPath))
		default:
			printSuccess("Vault Path", fmt.Sprintf("Accessible (%s)", vaultPath))
		}
	}

	// 2. Chroma Path
	chromaPath := globals.ChromaPath
	if chromaPath == "" {
		printError("ChromaDB Path", "Not configured.")
	} else {
		err := os.MkdirAll(chromaPath, 0755)
		if err != nil {
			printError("ChromaDB Path", fmt.Sprintf("Cannot create/access directory %q: %v", chromaPath, err))
		} else {
			testFile := filepath.Join(chromaPath, ".notebrain_test_write")
			err = os.WriteFile(testFile, []byte("test"), 0600)
			if err != nil {
				printError("ChromaDB Path", fmt.Sprintf("Directory %q is not writable: %v", chromaPath, err))
			} else {
				os.Remove(testFile)
				printSuccess("ChromaDB Path", fmt.Sprintf("Writable (%s)", chromaPath))
			}
		}
	}

	// 3. Tesseract OCR Dependency
	ocr := pdfextract.NewTesseractBackend("tesseract", "eng")
	if ocr.Available() {
		printSuccess("Tesseract OCR", "tesseract binary found in PATH")

		err := ocr.ValidateLang(context.Background())
		if err != nil {
			printWarning("Tesseract OCR Language", fmt.Sprintf("Validation failed: %v", err))
		} else {
			printSuccess("Tesseract OCR Language", "Default language data ('eng') is available")
		}
	} else {
		printWarning("Tesseract OCR", "tesseract binary not found in PATH. OCR for PDFs will not work.")
	}

	fmt.Println()
	fmt.Println("Run complete.")
	return nil
}

func printSuccess(check, msg string) {
	icon := scoreStyle.Render("[✓]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}

func printError(check, msg string) {
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	icon := errorStyle.Render("[✗]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}

func printWarning(check, msg string) {
	icon := warnScoreStyle.Render("[!]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}
