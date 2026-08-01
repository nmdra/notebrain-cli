package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// sqliteMagic is the 16-byte header every SQLite database file starts with.
const sqliteMagic = "SQLite format 3\x00"

// sqliteHealth inspects the chroma.sqlite3 file without opening it through
// the native library (which can abort on corrupted databases).
func sqliteHealth(chromaPath string) (exists bool, ok bool, detail string) {
	dbPath := filepath.Join(chromaPath, "chroma.sqlite3")
	info, err := os.Stat(dbPath)
	if err != nil {
		return false, false, "not found (database not initialized yet)"
	}
	if info.Size() == 0 {
		return true, false, "file is empty (truncated)"
	}

	f, err := os.Open(dbPath)
	if err != nil {
		return true, false, fmt.Sprintf("cannot read: %v", err)
	}
	defer f.Close()

	magic := make([]byte, len(sqliteMagic))
	if _, err := io.ReadFull(f, magic); err != nil {
		return true, false, "file too short to be a SQLite database"
	}
	if string(magic) != sqliteMagic {
		return true, false, "invalid SQLite header — file is not a valid database"
	}

	// Page size lives at byte offset 16 (big-endian u16; 1 means 65536).
	pageSize := 0
	pageBytes := make([]byte, 2)
	if _, err := f.ReadAt(pageBytes, 16); err == nil {
		if pageBytes[0] == 1 && pageBytes[1] == 0 {
			pageSize = 65536
		} else {
			pageSize = int(pageBytes[0])<<8 | int(pageBytes[1])
		}
	}
	if pageSize > 512 && info.Size()%int64(pageSize) != 0 {
		return true, false, fmt.Sprintf("size %d is not a multiple of the page size %d (truncated?)", info.Size(), pageSize)
	}

	return true, true, fmt.Sprintf("valid SQLite database (%d bytes)", info.Size())
}

// chromaSegmentFiles are the files every ChromaDB segment directory contains
// once its HNSW index has been persisted.
var chromaSegmentFiles = []string{
	"header.bin",
	"data_level0.bin",
	"length.bin",
	"link_lists.bin",
	"index_metadata.pickle",
}

// segmentIssues scans the segment directories under chromaPath and reports
// structural anomalies: missing or empty index files, typically left behind
// by an interrupted write (e.g. an abort during a batch sync).
func segmentIssues(chromaPath string) (segments int, issues []string) {
	entries, err := os.ReadDir(chromaPath)
	if err != nil {
		return 0, []string{fmt.Sprintf("cannot read %s: %v", chromaPath, err)}
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(chromaPath, e.Name())
		if _, err := os.Stat(filepath.Join(dir, "header.bin")); err != nil {
			continue // not a segment directory
		}
		segments++

		var missing []string
		var empty []string
		for _, name := range chromaSegmentFiles {
			fi, err := os.Stat(filepath.Join(dir, name))
			if err != nil {
				missing = append(missing, name)
				continue
			}
			if fi.Size() == 0 {
				empty = append(empty, name)
			}
		}
		if len(missing) > 0 {
			issues = append(issues, fmt.Sprintf("segment %s is incomplete: missing %s", e.Name(), strings.Join(missing, ", ")))
		}
		if len(empty) > 0 {
			issues = append(issues, fmt.Sprintf("segment %s has empty file(s): %s", e.Name(), strings.Join(empty, ", ")))
		}
	}
	return segments, issues
}

type probeResult struct {
	ok       bool
	signaled bool
	detail   string
}

// probeStoreOpen spawns a subprocess that opens the store and forces both
// HNSW indexes to load. A corrupted index aborts the subprocess with
// SIGABRT, which cannot be caught in-process, hence the subprocess.
func probeStoreOpen(chromaPath string) probeResult {
	exe, err := os.Executable()
	if err != nil {
		return probeResult{detail: fmt.Sprintf("cannot locate own executable: %v", err)}
	}
	return probeStoreOpenExec(chromaPath, exe)
}

func probeStoreOpenExec(chromaPath, exe string) probeResult {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "doctor-probe", "--chroma-path", chromaPath)
	out, err := cmd.CombinedOutput()

	switch {
	case err == nil:
		return probeResult{ok: true, detail: "opened collections and forced HNSW index load"}
	case ctx.Err() != nil:
		return probeResult{detail: "timed out after 90s (native loader hung)"}
	}
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			sig := ws.Signal()
			return probeResult{
				signaled: true,
				detail:   fmt.Sprintf("aborted by signal %d (%s) while opening — native HNSW index corrupted", int(sig), sig),
			}
		}
		return probeResult{detail: fmt.Sprintf("open failed (exit %d): %s", exitErr.ExitCode(), strings.TrimSpace(string(out)))}
	}
	return probeResult{detail: fmt.Sprintf("probe failed: %v", err)}
}
