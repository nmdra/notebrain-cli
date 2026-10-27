package embedder

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type repairEntry struct {
	name, body string
	kind       byte
}

func repairArchive(t *testing.T, entries ...repairEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		kind := e.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		if err := tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0600, Size: int64(len(e.body)), Typeflag: kind}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func repairPaths(t *testing.T) modelPaths {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, modelDirectoryName, "onnx")
	return modelPaths{cacheDir: root, modelPath: filepath.Join(dir, modelFileName), tokenizerPath: filepath.Join(dir, tokenizerFileName), lockPath: filepath.Join(root, lockFileName)}
}
func repairServer(t *testing.T, data []byte) (string, string) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	t.Cleanup(s.Close)
	return s.URL, fmt.Sprintf("%x", sha256.Sum256(data))
}
func assertRepairClean(t *testing.T, p modelPaths) {
	t.Helper()
	if _, err := os.Lstat(p.lockPath); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
	entries, err := os.ReadDir(p.cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != modelDirectoryName {
			t.Fatalf("temporary file remains: %s", e.Name())
		}
	}
}
func TestRepairModelInstallsArchiveLayout(t *testing.T) {
	p := repairPaths(t)
	data := repairArchive(t, repairEntry{"onnx/model.onnx", "model", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0}, repairEntry{"onnx/config.json", "ignored", 0})
	url, sum := repairServer(t, data)
	if err := repairModel(context.Background(), p, url, sum); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{p.modelPath: "model", p.tokenizerPath: "tokenizer"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("artifact %s = %q, %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.modelPath), "config.json")); !os.IsNotExist(err) {
		t.Fatal("unexpected artifact extracted")
	}
	assertRepairClean(t, p)
}

func TestRepairModelChecksumFailureLeavesCacheUntouched(t *testing.T) {
	p := repairPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.modelPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.modelPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	data := repairArchive(t, repairEntry{"onnx/model.onnx", "replacement", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0})
	url, _ := repairServer(t, data)
	if err := repairModel(context.Background(), p, url, "incorrect"); err == nil {
		t.Fatal("expected checksum failure")
	}
	got, err := os.ReadFile(p.modelPath)
	if err != nil || string(got) != "original" {
		t.Fatalf("existing artifact changed: %q %v", got, err)
	}
	if _, err := os.Stat(p.tokenizerPath); !os.IsNotExist(err) {
		t.Fatalf("tokenizer installed before checksum check: %v", err)
	}
	assertRepairClean(t, p)
}

func TestRepairModelRefusesExistingLocks(t *testing.T) {
	for _, contents := range []string{fmt.Sprint(os.Getpid()), "2147483647", "invalid"} {
		t.Run(contents, func(t *testing.T) {
			p := repairPaths(t)
			if err := os.WriteFile(p.lockPath, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			data := repairArchive(t, repairEntry{"onnx/model.onnx", "model", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0})
			url, sum := repairServer(t, data)
			if err := repairModel(context.Background(), p, url, sum); err == nil {
				t.Fatal("expected lock refusal")
			}
			got, err := os.ReadFile(p.lockPath)
			if err != nil || string(got) != contents {
				t.Fatalf("existing lock changed: %q %v", got, err)
			}
			if len(missingArtifacts(p)) != 2 {
				t.Fatal("cache artifacts changed while locked")
			}
		})
	}
}

func TestRepairModelHealthyDoesNotDownload(t *testing.T) {
	p := repairPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.modelPath), 0755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{p.modelPath, p.tokenizerPath} {
		if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A healthy model must work even when there is no reachable download endpoint.
	if err := repairModel(context.Background(), p, ":invalid-url", "unused"); err != nil {
		t.Fatal(err)
	}
	assertRepairClean(t, p)
}

func TestRepairModelPreservesHealthyArtifact(t *testing.T) {
	p := repairPaths(t)
	if err := os.MkdirAll(filepath.Dir(p.modelPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.modelPath, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	url, sum := repairServer(t, repairArchive(t, repairEntry{"onnx/model.onnx", "replacement", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0}))
	if err := repairModel(context.Background(), p, url, sum); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p.modelPath)
	if err != nil || string(got) != "original" {
		t.Fatalf("healthy model replaced: %q %v", got, err)
	}
	assertRepairClean(t, p)
}

func TestRepairModelRejectsUnsafeOrIncompleteArchive(t *testing.T) {
	model := repairEntry{"onnx/model.onnx", "model", 0}
	tokenizer := repairEntry{"onnx/tokenizer.json", "tokenizer", 0}
	tests := map[string][]repairEntry{
		"incomplete":   {model},
		"empty":        {model, {"onnx/tokenizer.json", "", 0}},
		"traversal":    {model, tokenizer, {"../../escape", "bad", 0}},
		"absolute":     {model, tokenizer, {"/tmp/escape", "bad", 0}},
		"symlink":      {model, {"onnx/tokenizer.json", "", tar.TypeSymlink}},
		"duplicate":    {model, tokenizer, model},
		"wrong layout": {{"unexpected/model.onnx", "model", 0}, tokenizer},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			p := repairPaths(t)
			url, sum := repairServer(t, repairArchive(t, entries...))
			if err := repairModel(context.Background(), p, url, sum); err == nil {
				t.Fatal("expected archive rejection")
			}
			if len(missingArtifacts(p)) != 2 {
				t.Fatal("invalid archive partially installed")
			}
			assertRepairClean(t, p)
		})
	}
}

func TestRepairModelRejectsHTTPError(t *testing.T) {
	p := repairPaths(t)
	data := repairArchive(t, repairEntry{"onnx/model.onnx", "model", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write(data)
	}))
	defer s.Close()
	err := repairModel(context.Background(), p, s.URL, fmt.Sprintf("%x", sha256.Sum256(data)))
	if err == nil {
		t.Fatal("expected HTTP status error")
	}
	if len(missingArtifacts(p)) != 2 {
		t.Fatal("HTTP error changed artifacts")
	}
	assertRepairClean(t, p)
}

func TestRepairModelCancellationCleansOwnedLock(t *testing.T) {
	p := repairPaths(t)
	ctx, cancel := context.WithCancel(context.Background())
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lock := inspectDownloadLock(p.lockPath, processIsRunning)
		if lock == nil || lock.Invalid || !lock.Active || lock.PID != os.Getpid() {
			t.Error("repair lock is not compatible with availability checks")
		}
		cancel()
		<-r.Context().Done()
	}))
	defer s.Close()
	defer cancel()
	if err := repairModel(ctx, p, s.URL, "unused"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	assertRepairClean(t, p)
}

func TestRepairModelDoesNotRemoveReplacementLock(t *testing.T) {
	p := repairPaths(t)
	data := repairArchive(t, repairEntry{"onnx/model.onnx", "model", 0}, repairEntry{"onnx/tokenizer.json", "tokenizer", 0})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := os.Remove(p.lockPath); err != nil {
			t.Error(err)
		}
		if err := os.WriteFile(p.lockPath, []byte("replacement"), 0600); err != nil {
			t.Error(err)
		}
		_, _ = w.Write(data)
	}))
	defer s.Close()
	if err := repairModel(context.Background(), p, s.URL, fmt.Sprintf("%x", sha256.Sum256(data))); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p.lockPath)
	if err != nil || string(got) != "replacement" {
		t.Fatalf("replacement lock removed: %q %v", got, err)
	}
}

func TestRepairModelPublicHealthyCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p, err := defaultModelPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p.modelPath), 0755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{p.modelPath, p.tokenizerPath} {
		if err := os.WriteFile(file, []byte("healthy"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := RepairModel(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertRepairClean(t, p)
}
