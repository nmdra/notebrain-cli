package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CheckExistingDatabase checks the existing SQLite file without loading Chroma.
// It is a header and file-size check, not a database integrity check.
func CheckExistingDatabase(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check existing database: %w", err)
	}
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open existing store: %w", err)
	}
	defer dir.Close()
	info, err := dir.Stat()
	if err != nil {
		return fmt.Errorf("stat existing store: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("existing store is not a directory")
	}
	f, err := os.Open(filepath.Join(path, "chroma.sqlite3"))
	if err != nil {
		return fmt.Errorf("open existing database: %w", err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return fmt.Errorf("stat existing database: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("existing database is not a nonempty regular file")
	}
	magic := make([]byte, 16)
	if _, err := io.ReadFull(f, magic); err != nil {
		return fmt.Errorf("read SQLite header: %w", err)
	}
	if string(magic) != "SQLite format 3\x00" {
		return fmt.Errorf("invalid SQLite header")
	}
	page := make([]byte, 2)
	if _, err := f.ReadAt(page, 16); err == nil {
		size := int(page[0])<<8 | int(page[1])
		if page[0] == 1 && page[1] == 0 {
			size = 65536
		}
		if size > 512 && info.Size()%int64(size) != 0 {
			return fmt.Errorf("database size %d is not a multiple of page size %d", info.Size(), size)
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("check existing database: %w", err)
	}
	return nil
}

// WithExistingDatabaseFile requires a valid existing SQLite file after locking.
// Unlike ordinary Open, it never creates a missing store directory.
func WithExistingDatabaseFile() Option {
	return func(s *Store) { s.existingDatabaseFile = true }
}
