package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Reserved internal child exit codes classify prerequisites without exposing diagnostics.
const (
	ExitStoreUnavailable   = 20
	ExitRuntimeUnavailable = 21
	ExitModelUnavailable   = 22
)

const maxStdout = 4 * 1024 * 1024
const maxStderr = 64 * 1024
const callTimeout = 90 * time.Second

// Runner runs only the retrieval allowlist in isolated CLI children.
// Close cancels and joins children before removing their private profiles.
type Runner struct {
	executable string
	globalArgs []string
	defaults   map[string]string
	dir        string
	mu         sync.Mutex
	closed     bool
	active     map[uint64]context.CancelFunc
	next       uint64
	wg         sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
}

// NewRunner snapshots trusted startup globals and creates a private profile directory.
func NewRunner(executable string, globalArgs []string, defaults map[string]string) (*Runner, error) {
	if executable == "" || !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("runner requires an absolute executable path")
	}
	dir, err := os.MkdirTemp("", "notebrain-mcp-")
	if err != nil {
		return nil, fmt.Errorf("create private profiles: %w", err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("protect private profiles: %w", err)
	}
	return &Runner{executable: executable, globalArgs: append([]string(nil), globalArgs...), defaults: clone(defaults), dir: dir, active: map[uint64]context.CancelFunc{}}, nil
}

// Execute reaps the child and removes its profile before returning. Child diagnostics are never reflected to the caller.
func (r *Runner) Execute(ctx context.Context, call Call) ([]byte, error) {
	effective := clone(r.defaults)
	maps.Copy(effective, call.Overrides)
	if err := validateCall(call, effective); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fmt.Errorf("runner is closed")
	}
	r.next++
	id := r.next
	r.active[id] = cancel
	r.wg.Add(1)
	r.mu.Unlock()
	defer func() { r.mu.Lock(); delete(r.active, id); r.mu.Unlock(); r.wg.Done() }()
	profile := map[string]any{}
	for k, v := range r.defaults {
		if allowed(call.Command, k) {
			profile[k] = v
		}
	}
	for k := range call.Overrides {
		delete(profile, k)
	}
	if call.Command == commandSearch {
		if value, ok := effective["exclude-notes"]; ok {
			var values []string
			if err := json.Unmarshal([]byte(value), &values); err != nil {
				return nil, fmt.Errorf("invalid exclusion profile")
			}
			profile["exclude-notes"] = values
		}
	}
	data, err := toml.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("encode private profile: %w", err)
	}
	f, err := os.CreateTemp(r.dir, "call-*.toml")
	if err != nil {
		return nil, fmt.Errorf("create private profile: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return nil, fmt.Errorf("write private profile: %w", err)
	}
	args := append([]string(nil), r.globalArgs...)
	args = append(args, "--mcp-child", "--config="+name, "--format=json", "--jsonpath=", call.Command)
	keys := make([]string, 0, len(call.Overrides))
	for k := range call.Overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := call.Overrides[k]
		if k == "exclude-notes" {
			continue
		}
		args = append(args, "--"+k+"="+v)
	}
	args = append(args, "--")
	args = append(args, call.Args...)
	cmd := exec.CommandContext(ctx, r.executable, args...) //nolint:gosec // G204: trusted executable; request values are allowlisted flags or positional values after --.
	cmd.Env = childEnvironment(os.Environ())
	cmd.Stdin = nil
	cmd.WaitDelay = 250 * time.Millisecond
	stdout := &boundedCapture{max: maxStdout, cancel: cancel}
	stderr := &boundedCapture{max: maxStderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if stdout.overflow {
		return nil, &executionError{message: "child output exceeded 4 MiB"}
	}
	if ctx.Err() != nil {
		return nil, &executionError{message: "retrieval canceled or timed out", cause: ctx.Err()}
	}
	if err != nil {
		return nil, classifyFailure(err)
	}
	return stdout.data, nil
}

// Close stops admission, cancels and joins active calls, then removes all profiles.
func (r *Runner) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		for _, cancel := range r.active {
			cancel()
		}
		r.mu.Unlock()
		r.wg.Wait()
		r.closeErr = os.RemoveAll(r.dir)
	})
	return r.closeErr
}

type boundedCapture struct {
	data     []byte
	max      int
	overflow bool
	cancel   context.CancelFunc
}

func (b *boundedCapture) Write(p []byte) (int, error) {
	n := len(p)
	left := b.max - len(b.data)
	if left > 0 {
		keep := min(len(p), left)
		b.data = append(b.data, p[:keep]...)
	}
	if n > left {
		b.overflow = true
		if b.cancel != nil {
			b.cancel()
		}
	}
	return n, nil
}

var _ io.Writer = (*boundedCapture)(nil)

func childEnvironment(env []string) []string {
	out := make([]string, 0, len(env))
	for _, s := range env {
		k, _, _ := strings.Cut(s, "=")
		u := strings.ToUpper(k)
		if u == "COMP_LINE" || u == "COMP_POINT" || u == "NOTEBRAIN_ALLOW_RUNTIME_DOWNLOAD" || strings.Contains(u, "API_KEY") || strings.HasPrefix(u, "NOTEBRAIN_LOG_") || strings.Contains(u, "HYPERLINK") || u == "TERM_PROGRAM" || u == "COLORTERM" || u == "WT_SESSION" {
			continue
		}
		out = append(out, s)
	}
	return append(out, "NO_HYPERLINKS=1")
}

// executionError contains only server-owned categorical diagnostics.
type executionError struct {
	message string
	cause   error
}

func (e *executionError) Error() string { return e.message }
func (e *executionError) Unwrap() error { return e.cause }
func classifyFailure(err error) error {
	message := "retrieval child failed"
	if exit, ok := errors.AsType[*exec.ExitError](err); ok {
		switch exit.ExitCode() {
		case -1:
			message = "retrieval child terminated by native signal"
		case ExitStoreUnavailable:
			message = "existing store is missing or invalid"
		case ExitRuntimeUnavailable:
			message = "required local runtime is unavailable"
		case ExitModelUnavailable:
			message = "required local model is unavailable"
		}
	}
	return &executionError{message: message, cause: err}
}
