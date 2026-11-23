//go:build linux

package store

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreLockAliasesAndCancellation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "store")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	first, canonical, err := acquireStoreLock(context.Background(), path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	if canonical != path {
		t.Fatalf("canonical=%q", canonical)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err = acquireStoreLock(ctx, alias, false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("alias did not contend: %v", err)
	}
	other, _, err := acquireStoreLock(context.Background(), filepath.Join(root, "other"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	if err := first.close(); err != nil {
		t.Fatal(err)
	}
	again, _, err := acquireStoreLock(context.Background(), alias, false)
	if err != nil {
		t.Fatal(err)
	}
	defer again.close()
}

func TestStoreLockMissingParentAndExistingOnly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a", "b", "store")
	if _, _, err := acquireStoreLock(context.Background(), path, true); err == nil {
		t.Fatal("existing-only accepted missing store")
	}
	if _, err := os.Stat(filepath.Join(root, "a")); !os.IsNotExist(err) {
		t.Fatalf("existing-only created parents: %v", err)
	}
	lock, canonical, err := acquireStoreLock(context.Background(), path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()
	if canonical != path {
		t.Fatalf("canonical=%q", canonical)
	}
}

// TestStoreLockProcessHelper exercises only OS locking, never native Chroma.
func TestStoreLockProcessHelper(t *testing.T) {
	if os.Getenv("NOTEBRAIN_LOCK_HELPER") != "1" {
		return
	}
	lock, _, err := acquireStoreLock(context.Background(), os.Getenv("NOTEBRAIN_LOCK_PATH"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()
	fmt.Println("locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestStoreLockProcessCrashRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	cmd := exec.Command(os.Args[0], "-test.run=^TestStoreLockProcessHelper$")
	cmd.Env = append(os.Environ(), "NOTEBRAIN_LOCK_HELPER=1", "NOTEBRAIN_LOCK_PATH="+path, "HOME="+t.TempDir())
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("helper readiness %q: %v", line, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := acquireStoreLock(ctx, path, false); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("different HOME did not contend: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	lock, _, err := acquireStoreLock(ctx2, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()
	info, err := lock.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("lock mode=%v", info.Mode())
	}
	if err := lock.close(); err != nil {
		t.Fatal(err)
	}
	again, _, err := acquireStoreLock(ctx2, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer again.close()
	next, err := again.file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(info, next) {
		t.Fatal("lock inode changed")
	}
}

func TestMCPPreflightOpenFailureReleasesLock(t *testing.T) {
	path := t.TempDir()
	if st, err := Open(context.Background(), path, WithExistingDatabaseFile()); err == nil || st != nil {
		t.Fatalf("Open=%v,%v", st, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lock, _, err := acquireStoreLock(ctx, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.close()
}

func TestStoreCloseIdempotent(t *testing.T) {
	lock, _, err := acquireStoreLock(context.Background(), filepath.Join(t.TempDir(), "store"), false)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{lifetimeLock: lock}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
