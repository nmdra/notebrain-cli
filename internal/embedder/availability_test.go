package embedder

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testModelPaths(t *testing.T) modelPaths {
	t.Helper()
	cacheDir := t.TempDir()
	modelDir := filepath.Join(cacheDir, "all-MiniLM-L6-v2", "onnx")
	return modelPaths{
		cacheDir:      cacheDir,
		modelPath:     filepath.Join(modelDir, "model.onnx"),
		tokenizerPath: filepath.Join(modelDir, "tokenizer.json"),
		lockPath:      filepath.Join(cacheDir, ".download.lock"),
	}
}

func writeModelArtifacts(t *testing.T, paths modelPaths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(paths.modelPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.modelPath, []byte("model"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.tokenizerPath, []byte("tokenizer"), 0600); err != nil {
		t.Fatal(err)
	}
}

func availabilityError(t *testing.T, err error) *ModelAvailabilityError {
	t.Helper()
	if err == nil {
		t.Fatal("expected model availability error")
	}
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("error %v does not unwrap to ErrModelUnavailable", err)
	}
	var availabilityErr *ModelAvailabilityError
	if !errors.As(err, &availabilityErr) {
		t.Fatalf("error %v is not a ModelAvailabilityError", err)
	}
	return availabilityErr
}

func TestCheckModelAvailability(t *testing.T) {
	t.Run("complete model", func(t *testing.T) {
		paths := testModelPaths(t)
		writeModelArtifacts(t, paths)
		if err := checkModelAvailability(paths, func(int) bool { return false }); err != nil {
			t.Fatalf("checkModelAvailability() error = %v", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.MkdirAll(filepath.Dir(paths.modelPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.tokenizerPath, []byte("tokenizer"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(int) bool { return false })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityMissingArtifacts {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityMissingArtifacts)
		}
		if !strings.Contains(err.Error(), paths.modelPath) {
			t.Errorf("error %q does not mention missing model path", err)
		}
	})

	t.Run("missing tokenizer", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.MkdirAll(filepath.Dir(paths.modelPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.modelPath, []byte("model"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(int) bool { return false })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityMissingArtifacts {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityMissingArtifacts)
		}
		if !strings.Contains(err.Error(), paths.tokenizerPath) {
			t.Errorf("error %q does not mention missing tokenizer path", err)
		}
	})

	t.Run("zero-byte artifact", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.MkdirAll(filepath.Dir(paths.modelPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.modelPath, nil, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(paths.tokenizerPath, []byte("tokenizer"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(int) bool { return false })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityMissingArtifacts {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityMissingArtifacts)
		}
		if len(availabilityErr.Missing) != 1 || availabilityErr.Missing[0] != paths.modelPath {
			t.Errorf("missing = %v, want [%s]", availabilityErr.Missing, paths.modelPath)
		}
	})

	t.Run("active download lock", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.WriteFile(paths.lockPath, []byte("12345"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(pid int) bool { return pid == 12345 })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityDownloadActive {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityDownloadActive)
		}
		if availabilityErr.Lock == nil || !availabilityErr.Lock.Active || availabilityErr.Lock.PID != 12345 {
			t.Errorf("lock = %+v, want active PID 12345", availabilityErr.Lock)
		}
	})

	t.Run("dead download lock", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.WriteFile(paths.lockPath, []byte("999999999"), 0600); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(int) bool { return false })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityDownloadStale {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityDownloadStale)
		}
		if availabilityErr.Lock == nil || availabilityErr.Lock.Active || availabilityErr.Lock.PID != 999999999 {
			t.Errorf("lock = %+v, want inactive PID 999999999", availabilityErr.Lock)
		}
	})

	t.Run("unreadable lock metadata", func(t *testing.T) {
		paths := testModelPaths(t)
		if err := os.Mkdir(paths.lockPath, 0700); err != nil {
			t.Fatal(err)
		}

		err := checkModelAvailability(paths, func(int) bool { return false })
		availabilityErr := availabilityError(t, err)
		if availabilityErr.Reason != AvailabilityInvalidLock {
			t.Errorf("reason = %q, want %q", availabilityErr.Reason, AvailabilityInvalidLock)
		}
		if availabilityErr.Lock == nil || !availabilityErr.Lock.Invalid {
			t.Errorf("lock = %+v, want invalid lock metadata", availabilityErr.Lock)
		}
	})
}

func TestModelAvailabilityErrorMessage(t *testing.T) {
	paths := testModelPaths(t)
	err := checkModelAvailability(paths, func(int) bool { return false })
	message := err.Error()
	for _, want := range []string{
		"ONNX embedding model is unavailable",
		paths.modelPath,
		paths.tokenizerPath,
		"notebrain doctor",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not contain %q", message, want)
		}
	}
}
