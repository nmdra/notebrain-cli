package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
)

func TestPersistentClientOptionsOptInAcceptsEmptyLibraryPath(t *testing.T) {
	t.Setenv("CHROMA_LIB_PATH", "")
	options := persistentClientOptions(t.TempDir(), "", true)
	// Override download permission last so resolution stops before any network
	// access. The resolver error proves every assembled option was accepted.
	options = append(options, chroma.WithPersistentLibraryAutoDownload(false))
	client, err := chroma.NewPersistentClient(options...)
	if client != nil {
		_ = client.Close()
		t.Fatal("unexpected client")
	}
	if err == nil || !strings.Contains(err.Error(), "local runtime library path is not configured") {
		t.Fatalf("want resolver error after valid options, got %v", err)
	}
}

func TestOpenInvalidExplicitRuntimeDoesNotDownload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-runtime.so")
	t.Setenv("CHROMA_LIB_PATH", path)
	t.Setenv(runtimeDownloadEnv, "1")
	st, err := Open(context.Background(), t.TempDir())
	var unavailable *RuntimeAvailabilityError
	if st != nil || !errors.As(err, &unavailable) || !unavailable.Explicit || unavailable.Path != path {
		t.Fatalf("Open = %v, %v; want explicit runtime availability error", st, err)
	}
}
