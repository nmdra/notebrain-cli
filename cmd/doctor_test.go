package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"

	"github.com/nmdra/notebrain-cli/v2/internal/configfile"
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

func TestDoctorCmdFixMissingModel(t *testing.T) {
	withModelAvailability(t, &embedder.ModelAvailabilityError{
		Reason:  embedder.AvailabilityMissingArtifacts,
		Missing: []string{"model.onnx", "tokenizer.json"},
	})
	cmd := &DoctorCmd{Fix: true, repairModel: func(ctx context.Context) error {
		checkModelAvailabilityFn = func() error { return nil }
		return nil
	}}
	out := captureStdout(t, func() {
		if err := cmd.Run(&Globals{Ctx: context.Background(), ChromaPath: t.TempDir()}); err != nil {
			t.Fatalf("doctor --fix: %v", err)
		}
	})
	if !strings.Contains(out, "ONNX model: available") {
		t.Fatalf("repair did not report available model: %s", out)
	}
}

func TestDoctorCmdFixSafety(t *testing.T) {
	missing := &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityMissingArtifacts, Missing: []string{"model.onnx"}}
	for _, tt := range []struct {
		name         string
		fix          bool
		availability error
		repairErr    error
		wantRepair   bool
		wantErr      bool
		wantOutput   string
	}{
		{name: "diagnostic only", availability: missing, wantErr: true, wantOutput: "notebrain doctor --fix"},
		{name: "healthy", fix: true, wantOutput: "ONNX model: available"},
		{name: "active lock", fix: true, availability: &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityDownloadActive}, wantErr: true, wantOutput: "do not remove its lock"},
		{name: "stale lock", fix: true, availability: &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityDownloadStale}, wantErr: true, wantOutput: "confirm the recorded PID"},
		{name: "invalid lock", fix: true, availability: &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityInvalidLock}, wantErr: true, wantOutput: "inspect the lock metadata"},
		{name: "download failure", fix: true, availability: missing, repairErr: errors.New("download failed: offline"), wantRepair: true, wantErr: true, wantOutput: "download failed: offline"},
		{name: "repair did not restore files", fix: true, availability: missing, wantRepair: true, wantErr: true, wantOutput: "ONNX embedding model is unavailable"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			withModelAvailability(t, tt.availability)
			called := false
			cmd := &DoctorCmd{Fix: tt.fix, repairModel: func(context.Context) error {
				called = true
				return tt.repairErr
			}}
			out := captureStdout(t, func() {
				err := cmd.Run(&Globals{Ctx: context.Background(), ChromaPath: t.TempDir()})
				if (err != nil) != tt.wantErr {
					t.Errorf("error = %v, want error: %v", err, tt.wantErr)
				}
			})
			if called != tt.wantRepair {
				t.Errorf("repair called = %v, want %v", called, tt.wantRepair)
			}
			if !strings.Contains(out, tt.wantOutput) {
				t.Errorf("output %q does not contain %q", out, tt.wantOutput)
			}
		})
	}
}

func TestDoctorCmdFixFlag(t *testing.T) {
	var cli struct {
		Doctor DoctorCmd `cmd:""`
	}
	resolver, err := configfile.TOMLResolver(strings.NewReader("fix = true\nlimit = 17\n"))
	if err != nil {
		t.Fatal(err)
	}
	parser, err := kong.New(&cli, kong.Resolvers(resolver))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"doctor"}); err != nil {
		t.Fatal(err)
	}
	if cli.Doctor.Fix {
		t.Fatal("TOML enabled repair without an explicit --fix flag")
	}
	if _, err := parser.Parse([]string{"doctor", "--fix"}); err != nil {
		t.Fatal(err)
	}
	if !cli.Doctor.Fix {
		t.Fatal("doctor --fix did not enable model repair")
	}
}

func TestDoctorCmdFixCanceled(t *testing.T) {
	withModelAvailability(t, &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityMissingArtifacts})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "chroma.sqlite3"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	origProbe := probeStoreOpenFn
	probeStoreOpenFn = func(string) probeResult {
		t.Error("canceled repair must not start database probe")
		return probeResult{ok: true}
	}
	t.Cleanup(func() { probeStoreOpenFn = origProbe })
	cmd := &DoctorCmd{Fix: true, repairModel: func(got context.Context) error {
		if got != ctx {
			t.Error("repair did not receive caller context")
		}
		return got.Err()
	}}
	if err := cmd.Run(&Globals{Ctx: ctx, ChromaPath: dir}); !errors.Is(err, context.Canceled) {
		t.Fatalf("want cancellation error, got %v", err)
	}
}

func TestDoctorCmdFixFailureWithInvalidChromaPath(t *testing.T) {
	withModelAvailability(t, &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityMissingArtifacts})
	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := &DoctorCmd{Fix: true, repairModel: func(context.Context) error { return errors.New("offline") }}
	if err := cmd.Run(&Globals{Ctx: context.Background(), ChromaPath: path}); err == nil {
		t.Fatal("failed repair with invalid Chroma path must fail")
	}
}

func TestDoctorCmdCorruptDatabase(t *testing.T) {
	withModelAvailability(t, &embedder.ModelAvailabilityError{Reason: embedder.AvailabilityMissingArtifacts})
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
	repaired := false
	cmd := &DoctorCmd{Fix: true, repairModel: func(context.Context) error {
		repaired = true
		checkModelAvailabilityFn = func() error { return nil }
		return nil
	}}
	err := cmd.Run(globals)
	if !repaired {
		t.Fatal("expected model repair before database checks")
	}
	if err == nil {
		t.Fatal("expected doctor to report the corrupt database")
	}
	if !strings.Contains(err.Error(), "problem(s) found") {
		t.Errorf("error %q should mention the problem count", err)
	}
	data, readErr := os.ReadFile(filepath.Join(dir, "chroma.sqlite3"))
	if readErr != nil || string(data) != "garbage-not-sqlite" {
		t.Fatalf("doctor --fix changed the database: %q, %v", data, readErr)
	}
}
