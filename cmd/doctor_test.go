package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nmdra/notebrain-cli/v2/internal/embedder"
)

func withModelAvailability(t *testing.T, err error) {
	t.Helper()
	orig := checkModelAvailabilityFn
	checkModelAvailabilityFn = func() error { return err }
	t.Cleanup(func() { checkModelAvailabilityFn = orig })
}

func TestDoctorCmdFreshVault(t *testing.T) {
	withModelAvailability(t, nil)
	globals := &Globals{
		Ctx:        context.Background(),
		ChromaPath: t.TempDir(),
	}
	if err := (&DoctorCmd{}).Run(globals); err != nil {
		t.Fatalf("doctor on fresh vault: %v", err)
	}
}

func TestDoctorCmdUnconfiguredChroma(t *testing.T) {
	globals := &Globals{Ctx: context.Background()}
	if err := (&DoctorCmd{}).Run(globals); err != nil {
		t.Fatalf("doctor without chroma path: %v", err)
	}
}

func TestDoctorCmdReportsMissingModel(t *testing.T) {
	withModelAvailability(t, &embedder.ModelAvailabilityError{
		Reason:        embedder.AvailabilityMissingArtifacts,
		ModelPath:     "/tmp/onnx/model.onnx",
		TokenizerPath: "/tmp/onnx/tokenizer.json",
		LockPath:      "/tmp/onnx/.download.lock",
		Missing:       []string{"/tmp/onnx/model.onnx", "/tmp/onnx/tokenizer.json"},
	})
	origProbe := probeStoreOpenFn
	probeStoreOpenFn = func(string) probeResult { return probeResult{ok: true} }
	t.Cleanup(func() { probeStoreOpenFn = origProbe })

	out := captureStdout(t, func() {
		err := (&DoctorCmd{}).Run(&Globals{Ctx: context.Background(), ChromaPath: t.TempDir()})
		if err == nil {
			t.Error("expected doctor to report missing model")
		}
	})
	for _, want := range []string{"ONNX model", "missing", "restore the missing files"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output %q does not contain %q", out, want)
		}
	}
}

func TestDoctorCmdCorruptDatabase(t *testing.T) {
	withModelAvailability(t, nil)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chroma.sqlite3"), []byte("garbage-not-sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	origProbe := probeStoreOpenFn
	probeStoreOpenFn = func(string) probeResult {
		return probeResult{detail: "stubbed probe failure"}
	}
	t.Cleanup(func() { probeStoreOpenFn = origProbe })

	globals := &Globals{
		Ctx:        context.Background(),
		ChromaPath: dir,
	}
	err := (&DoctorCmd{}).Run(globals)
	if err == nil {
		t.Fatal("expected doctor to report the corrupt database")
	}
	if !strings.Contains(err.Error(), "problem(s) found") {
		t.Errorf("error %q should mention the problem count", err)
	}
}
