//go:build !linux

package store

import (
	"context"
	"fmt"
	"os"
)

func openStoreLockFile(string) (*os.File, error) {
	return nil, fmt.Errorf("store lifetime locking is unsupported on this platform")
}
func lockStoreFile(context.Context, *os.File) error {
	return fmt.Errorf("store lifetime locking is unsupported on this platform")
}
func unlockStoreFile(*os.File) error {
	return fmt.Errorf("store lifetime locking is unsupported on this platform")
}
