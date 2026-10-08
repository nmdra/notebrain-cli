package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRuntimeLibrary(t *testing.T) {
	for _, tc := range []struct {
		name, explicit, optIn, content                string
		installed, directory, wantDownload, wantError bool
	}{
		{name: "missing", wantError: true},
		{name: "opt in", optIn: "1", wantDownload: true},
		{name: "opt in must be exact", optIn: "true", wantError: true},
		{name: "installed", installed: true, content: "runtime"},
		{name: "installed opt in stays quiet", installed: true, content: "runtime", optIn: "1"},
		{name: "empty", installed: true, wantError: true},
		{name: "directory", directory: true, wantError: true},
		{name: "explicit missing", explicit: "custom.so", wantError: true},
		{name: "explicit invalid opt in", explicit: "custom.so", optIn: "1", wantError: true},
		{name: "explicit installed", explicit: "custom.so", installed: true, content: "runtime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".cache", "chroma", "local_shim", defaultRuntimeVersion, "linux-amd64", "libchroma_shim.so")
			explicit := ""
			if tc.explicit != "" {
				path = filepath.Join(home, tc.explicit)
				explicit = "  " + path + "  "
			}
			if tc.installed || tc.directory {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if tc.directory {
					if err := os.Mkdir(path, 0700); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, download, err := resolveRuntimeLibrary(explicit, tc.optIn, func() (string, error) { return home, nil }, "linux", "amd64")
			if (err != nil) != tc.wantError || download != tc.wantDownload {
				t.Fatalf("resolve = %q, %v, %v", got, download, err)
			}
			if err == nil && !download && got != path {
				t.Fatalf("path = %q, want %q", got, path)
			}
			if err != nil {
				if !errors.Is(err, ErrRuntimeUnavailable) {
					t.Fatalf("not availability error: %v", err)
				}
				if !strings.Contains(err.Error(), path) {
					t.Fatalf("missing path: %v", err)
				}
				if explicit == "" && (!strings.Contains(err.Error(), "NOTEBRAIN_ALLOW_RUNTIME_DOWNLOAD=1 notebrain stats --format=json") || !strings.Contains(err.Error(), ".download.lock")) {
					t.Fatalf("missing recovery guidance: %v", err)
				}
			}
		})
	}
}

func TestRuntimePlatformsAndConfiguration(t *testing.T) {
	for _, tc := range []struct{ os, arch, file string }{
		{"darwin", "arm64", "libchroma_shim.dylib"},
		{"windows", "amd64", "chroma_shim.dll"},
	} {
		home := t.TempDir()
		_, _, err := resolveRuntimeLibrary("", "", func() (string, error) { return home, nil }, tc.os, tc.arch)
		if !strings.Contains(err.Error(), filepath.Join(tc.os+"-"+tc.arch, tc.file)) {
			t.Fatalf("wrong platform path: %v", err)
		}
	}
	_, _, err := resolveRuntimeLibrary("", "1", func() (string, error) { return "", errors.New("no home") }, "linux", "amd64")
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("home error: %v", err)
	}
	_, _, err = resolveRuntimeLibrary("", "1", nil, "linux", "arm64")
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("platform error: %v", err)
	}
}

func TestOpenMissingRuntimeFailsBeforeClient(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CHROMA_LIB_PATH", "")
	t.Setenv(runtimeDownloadEnv, "")
	path := filepath.Join(home, "database")
	st, err := Open(context.Background(), path)
	if st != nil || !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("Open = %v, %v", st, err)
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Open created cache or database: %v, %v", entries, err)
	}
}

func TestExplicitRuntimeLoaderCandidates(t *testing.T) {
	for _, tc := range []struct{ name, configured, artifact string }{
		{"directory", ".", "libchroma_shim.so"},
		{"extensionless file", "shim", "shim"},
		{"added extension", "shim", "shim.so"},
		{"added lib prefix", "shim", "libshim.so"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, tc.artifact), []byte("runtime"), 0600); err != nil {
				t.Fatal(err)
			}
			configured := filepath.Join(dir, tc.configured)
			path, download, err := resolveRuntimeLibrary(configured, "1", nil, "linux", "amd64")
			if err != nil || download || path != configured {
				t.Fatalf("resolve = %q, %v, %v", path, download, err)
			}
		})
	}
	path, download, err := resolveRuntimeLibrary("libchroma_shim.so", "1", nil, "linux", "amd64")
	if err != nil || download || path != "libchroma_shim.so" {
		t.Fatalf("bare loader name = %q, %v, %v", path, download, err)
	}
}
