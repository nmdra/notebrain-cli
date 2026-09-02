package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
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
		return nil
	}

	hardFailures := 0

	// Check the model before the native store probe. The probe intentionally
	// opens Chroma without initializing the semantic embedder, so doctor can
	// report both database and model problems without hanging in a downloader.
	modelErr := checkModelAvailabilityFn()
	if modelErr == nil {
		printSuccess("ONNX model", "available")
	} else {
		hardFailures++
		availabilityErr, ok := errors.AsType[*embedder.ModelAvailabilityError](modelErr)
		if ok {
			printError("ONNX model", availabilityErr.Error())
			switch availabilityErr.Reason {
			case embedder.AvailabilityDownloadActive:
				printWarning("ONNX model recovery", "wait for the active download; do not remove its lock")
			case embedder.AvailabilityDownloadStale:
				printWarning("ONNX model recovery", "confirm the recorded PID is not running, then remove the stale lock and restore the model files")
			case embedder.AvailabilityInvalidLock:
				printWarning("ONNX model recovery", "inspect the lock metadata and remove it only after confirming no download is active")
			default:
				printWarning("ONNX model recovery", "restore the missing files, then rerun doctor")
			}
		} else {
			printError("ONNX model", modelErr.Error())
		}
	}

	err := os.MkdirAll(chromaPath, 0755)
	if err != nil {
		printError("ChromaDB Path", fmt.Sprintf("Cannot create/access directory %q: %v", chromaPath, err))
		return nil
	}
	testFile := filepath.Join(chromaPath, ".notebrain_test_write")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		printError("ChromaDB Path", fmt.Sprintf("Directory %q is not writable: %v", chromaPath, err))
		return nil
	}
	os.Remove(testFile)
	printSuccess("ChromaDB Path", fmt.Sprintf("Writable (%s)", chromaPath))

	// 3. ChromaDB sqlite file
	sqliteExists, sqliteOK, sqliteDetail := sqliteHealth(chromaPath)
	switch {
	case !sqliteExists:
		printWarning("ChromaDB sqlite", sqliteDetail)
	case !sqliteOK:
		hardFailures++
		printError("ChromaDB sqlite", sqliteDetail)
	default:
		printSuccess("ChromaDB sqlite", sqliteDetail)
	}

	// 4. Collection segments
	segments, segIssues := segmentIssues(chromaPath)
	switch {
	case segments == 0:
		printWarning("ChromaDB index", "No collection segments found (nothing ingested yet?).")
	case len(segIssues) > 0:
		for _, issue := range segIssues {
			hardFailures++
			printError("ChromaDB index", issue)
		}
	default:
		printSuccess("ChromaDB index", fmt.Sprintf("%d collection segment(s) look structurally intact.", segments))
	}

	// 5. Open test: the definitive check. A corrupted HNSW index aborts the
	// probe subprocess with a signal instead of returning a Go error.
	if sqliteExists || segments > 0 {
		res := probeStoreOpenFn(chromaPath)
		if res.ok {
			printSuccess("ChromaDB open test", res.detail)
		} else {
			hardFailures++
			if res.signaled {
				printError("ChromaDB open test", res.detail)
				printWarning("Recovery", "Run 'notebrain reset' and re-ingest the vault.")
			} else {
				printError("ChromaDB open test", res.detail)
			}
		}
	} else {
		printWarning("ChromaDB open test", "Skipped (database not initialized).")
	}

	fmt.Println()
	if hardFailures > 0 {
		return fmt.Errorf("doctor: %d problem(s) found", hardFailures)
	}
	fmt.Println("Run complete.")
	return nil
}

func printSuccess(check, msg string) {
	icon := scoreStyle.Render("[✓]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}

func printError(check, msg string) {
	icon := errorStyle.Render("[✗]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}

func printWarning(check, msg string) {
	icon := warnScoreStyle.Render("[!]")
	fmt.Printf("%s %s: %s\n", icon, check, msg)
}
