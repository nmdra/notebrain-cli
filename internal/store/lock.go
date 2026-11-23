package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

type storeLock struct {
	file *os.File
	once sync.Once
	err  error
}

func (l *storeLock) close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() { l.err = errors.Join(unlockStoreFile(l.file), l.file.Close()) })
	return l.err
}

// canonicalStorePath resolves aliases, including paths below missing parents.
func canonicalStorePath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("absolute store path: %w", err)
	}
	ancestor := absolute
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			for _, m := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, m)
			}
			return resolved, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve store path: %w", err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("resolve store path: %w", err)
		}
		missing = append(missing, filepath.Base(ancestor))
		ancestor = parent
	}
}

func acquireStoreLock(ctx context.Context, path string, existingOnly bool) (*storeLock, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", fmt.Errorf("acquire store lock: %w", err)
	}
	canonical, err := canonicalStorePath(path)
	if err != nil {
		return nil, "", err
	}
	if existingOnly {
		info, statErr := os.Stat(canonical) // #nosec G703 -- Canonical operator-configured store path; existing-only validation must inspect this directory.
		if statErr != nil {
			return nil, "", fmt.Errorf("existing store: %w", statErr)
		}
		if !info.IsDir() {
			return nil, "", fmt.Errorf("existing store is not a directory")
		}
	} else if mkdirErr := os.MkdirAll(filepath.Dir(canonical), 0700); mkdirErr != nil { // #nosec G703 -- Ordinary CLI explicitly permits creation below its canonical operator-configured store parent.
		return nil, "", fmt.Errorf("create store parent: %w", mkdirErr)
	}
	name := fmt.Sprintf(".notebrain-store-%x.lock", sha256.Sum256([]byte(canonical)))
	file, err := openStoreLockFile(filepath.Join(filepath.Dir(canonical), name))
	if err != nil {
		return nil, "", fmt.Errorf("open store lock: %w", err)
	}
	if err := lockStoreFile(ctx, file); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("acquire store lock: %w", err)
	}
	lock := &storeLock{file: file}
	if err := ctx.Err(); err != nil {
		return nil, "", errors.Join(fmt.Errorf("acquire store lock: %w", err), lock.close())
	}
	return lock, canonical, nil
}
