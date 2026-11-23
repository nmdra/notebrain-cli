package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func buildMCPBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notebrain")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-mod=vendor", "-o", path, ".") // #nosec G204 -- fixed test build; output is in a private temp directory.
	build.Env = append(os.Environ(), "CGO_ENABLED=1")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build notebrain: %v\n%s", err, out)
	}
	return path
}

// Each server and CLI subprocess gets disposable paths and no download opt-in.
func mcpProcessEnv(t *testing.T) (env []string, home, tmp string) {
	t.Helper()
	home, tmp = t.TempDir(), t.TempDir()
	env = append(os.Environ(), "HOME="+home, "TMPDIR="+tmp, "XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"NOTEBRAIN_ALLOW_RUNTIME_DOWNLOAD=0", "CHROMA_LIB_PATH="+filepath.Join(home, "absent-chroma.so"),
		"COMP_LINE=", "COMP_POINT=", "DEEPSEEK_API_KEY=", "OPENROUTER_API_KEY=")
	return env, home, tmp
}

type mcpProcess struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *os.File
	done    chan struct{}
	waitErr error // published when done closes
	wire    bytes.Buffer
	stderr  bytes.Buffer
}

// Capture stdout independently of the SDK, including bytes written after it
// closes its reader. An error forwarding bytes cannot stop capture.
type mcpPipeSink struct{ io.Writer }

func (s mcpPipeSink) Write(p []byte) (int, error) {
	_, _ = s.Writer.Write(p)
	return len(p), nil
}

func startMCPProcess(t *testing.T, bin string, env []string, args ...string) *mcpProcess {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), bin, args...) // #nosec G204 -- runs only the executable built by this test.
	cmd.Env = env
	cmd.WaitDelay = 3 * time.Second
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		stdin.Close()
		t.Fatal(err)
	}
	p := &mcpProcess{cmd: cmd, stdin: stdin, stdout: reader, done: make(chan struct{})}
	cmd.Stdout = io.MultiWriter(&p.wire, mcpPipeSink{writer})
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		stdin.Close()
		reader.Close()
		writer.Close()
		t.Fatal(err)
	}
	go func() {
		p.waitErr = cmd.Wait()
		writer.Close()
		close(p.done)
	}()
	t.Cleanup(func() {
		p.stdin.Close()
		p.stdout.Close()
		_ = p.cmd.Process.Kill()
		p.wait(t)
		assertMCPWire(t, p.wire.Bytes())
	})
	return p
}

func (p *mcpProcess) wait(t *testing.T) error {
	t.Helper()
	select {
	case <-p.done:
		return p.waitErr
	case <-time.After(8 * time.Second):
		t.Fatal("notebrain process did not exit")
		return nil
	}
}

func (p *mcpProcess) connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "notebrain-process-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: p.stdout, Writer: p.stdin}, nil)
	if err != nil {
		t.Fatalf("connect SDK to notebrain: %v", err)
	}
	return session
}

func (p *mcpProcess) shutdown(t *testing.T, sig os.Signal) {
	t.Helper()
	if sig == nil {
		if err := p.stdin.Close(); err != nil {
			t.Fatal(err)
		}
	} else if err := p.cmd.Process.Signal(sig); err != nil {
		t.Fatal(err)
	}
	if err := p.wait(t); err != nil {
		t.Fatalf("server shutdown: %v\n%s", err, p.stderr.String())
	}
}

func assertMCPWire(t *testing.T, wire []byte) {
	t.Helper()
	if len(wire) == 0 {
		return
	}
	if wire[len(wire)-1] != '\n' {
		t.Error("protocol stdout has a partial line")
	}
	for line := range bytes.SplitSeq(bytes.TrimSuffix(wire, []byte("\n")), []byte("\n")) {
		var msg struct {
			JSONRPC string `json:"jsonrpc"`
		}
		if err := json.Unmarshal(line, &msg); err != nil || msg.JSONRPC != "2.0" {
			t.Errorf("non-protocol stdout line: %q", line)
		}
	}
}

func assertMCPDiscovery(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertMCPToolNames(t, list)
}

func assertMCPToolNames(t *testing.T, list *mcp.ListToolsResult) {
	t.Helper()
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{"notebrain_backlinks", "notebrain_boosted", "notebrain_connections", "notebrain_get", "notebrain_hidden", "notebrain_refs", "notebrain_search", "notebrain_stats", "notebrain_tags"}
	if !slices.Equal(names, want) || list.NextCursor != "" {
		t.Fatalf("tools/list = %q (cursor %q), want exactly %q", names, list.NextCursor, want)
	}
}

func assertMCPTempEmpty(t *testing.T, tmp string) {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("server left temporary entries: %v", entries)
	}
}

func TestMCPProcess(t *testing.T) {
	bin := buildMCPBinary(t)
	t.Run("sdk_over_exact_user_command", func(t *testing.T) {
		env, _, tmp := mcpProcessEnv(t)
		// Bash completion must not intercept the stdio command.
		env = append(env, "COMP_LINE=notebrain get ", "COMP_POINT=14")
		p := startMCPProcess(t, bin, env, "mcp", "serve")
		session := p.connect(t)
		init := session.InitializeResult()
		if init.ProtocolVersion != "2026-07-28" || init.ServerInfo == nil || init.ServerInfo.Name != "notebrain" || init.ServerInfo.Version == "" {
			t.Fatalf("SDK negotiation: %+v", init)
		}
		if caps := init.Capabilities; caps == nil || caps.Tools == nil || caps.Resources != nil || caps.Prompts != nil {
			t.Fatalf("unexpected server capabilities: %+v", caps)
		}
		assertMCPDiscovery(t, session)
		p.shutdown(t, nil)
		assertMCPTempEmpty(t, tmp)
	})
	t.Run("bare_command_shows_help", func(t *testing.T) {
		env, _, tmp := mcpProcessEnv(t)
		p := startMCPProcess(t, bin, env, "mcp")
		if err := p.wait(t); err != nil || p.wire.Len() != 0 || !strings.Contains(p.stderr.String(), "serve") {
			t.Fatalf("bare command did not show help on stderr: %v, stdout=%q, stderr=%q", err, &p.wire, &p.stderr)
		}
		assertMCPTempEmpty(t, tmp)
	})
	t.Run("legacy_initialize_and_signal", func(t *testing.T) {
		env, _, tmp := mcpProcessEnv(t)
		p := startMCPProcess(t, bin, env, "mcp", "serve")
		read := bufio.NewReader(p.stdout)
		legacyExchange(t, p, read, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"legacy-test","version":"1"}}}`, "init", func(raw json.RawMessage) {
			var init mcp.InitializeResult
			if err := json.Unmarshal(raw, &init); err != nil || init.ProtocolVersion != "2025-11-25" {
				t.Fatalf("legacy initialization: %s (%v)", raw, err)
			}
		})
		if _, err := fmt.Fprintln(p.stdin, `{"jsonrpc":"2.0","method":"notifications/initialized"}`); err != nil {
			t.Fatal(err)
		}
		legacyExchange(t, p, read, `{"jsonrpc":"2.0","id":"list","method":"tools/list","params":{}}`, "list", func(raw json.RawMessage) {
			var list mcp.ListToolsResult
			if err := json.Unmarshal(raw, &list); err != nil {
				t.Fatal(err)
			}
			assertMCPToolNames(t, &list)
		})
		p.shutdown(t, syscall.SIGTERM)
		assertMCPTempEmpty(t, tmp)
	})
	t.Run("malformed_config_stderr_only", func(t *testing.T) {
		env, home, tmp := mcpProcessEnv(t)
		config := filepath.Join(home, "invalid.toml")
		mcpWriteFile(t, config, "[broken\n")
		p := startMCPProcess(t, bin, env, "mcp", "serve", "--format=json", "--config="+config)
		if err := p.wait(t); err == nil {
			t.Fatal("malformed config did not fail startup")
		}
		if p.wire.Len() != 0 || !strings.Contains(p.stderr.String(), "failed to parse TOML") {
			t.Fatalf("malformed config: stdout=%q stderr=%q", p.wire.String(), p.stderr.String())
		}
		assertMCPTempEmpty(t, tmp)
	})
	t.Run("missing_store_is_a_tool_error_without_creation", func(t *testing.T) {
		env, home, tmp := mcpProcessEnv(t)
		missing := filepath.Join(home, "never-created")
		p := startMCPProcess(t, bin, env, "mcp", "serve", "--chroma-path="+missing)
		session := p.connect(t)
		ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
		defer cancel()
		res, err := session.CallTool(ctx, mcpCall("stats", nil))
		assertMCPToolError(t, res, err, "store")
		assertMCPDiscovery(t, session)
		p.shutdown(t, nil)
		if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing store was created or cannot be checked: %v", err)
		}
		entries, err := filepath.Glob(filepath.Join(home, ".notebrain-store-*.lock"))
		if err != nil || len(entries) != 0 {
			t.Fatalf("missing store created a sibling lock: %v (%v)", entries, err)
		}
		assertMCPTempEmpty(t, tmp)
	})
	t.Run("cross_process_lock", func(t *testing.T) { testMCPProcessLock(t, bin) })
}

func legacyExchange(t *testing.T, p *mcpProcess, read *bufio.Reader, request, id string, assertResult func(json.RawMessage)) {
	t.Helper()
	if _, err := fmt.Fprintln(p.stdin, request); err != nil {
		t.Fatal(err)
	}
	if err := p.stdout.SetReadDeadline(time.Now().Add(8 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
	}
	line, err := read.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(line, &response); err != nil || response.ID != id || len(response.Result) == 0 {
		t.Fatalf("response for %q: %s (%v)", id, line, err)
	}
	assertResult(response.Result)
}

func mcpWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mcpCall(tool string, args map[string]any) *mcp.CallToolParams {
	if args == nil {
		args = map[string]any{}
	}
	return &mcp.CallToolParams{Name: "notebrain_" + tool, Arguments: args}
}

func assertMCPToolError(t *testing.T, result *mcp.CallToolResult, err error, diagnostic string) {
	t.Helper()
	if err != nil || result == nil || !result.IsError || result.StructuredContent != nil {
		t.Fatalf("expected only a tool error, got %+v (%v)", result, err)
	}
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok && text.Text != "" && strings.Contains(strings.ToLower(text.Text), diagnostic) {
			return
		}
	}
	t.Errorf("expected tool error to contain %q, got %+v", diagnostic, result.Content)
}

// Use the published lock filename contract, independent of the store package.
func mcpLockPath(t *testing.T, store string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(store)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(canonical), fmt.Sprintf(".notebrain-store-%x.lock", sha256.Sum256([]byte(canonical))))
}

func startMCPFlock(t *testing.T, lockPath string) *exec.Cmd {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("process lock test requires Linux")
	}
	flock, err := exec.LookPath("flock")
	if err != nil {
		t.Skip("process lock test requires util-linux flock")
	}
	// --no-fork keeps the lock in the cat process we can kill. cat echoes
	// the readiness marker only after flock acquires the lock.
	cmd := exec.CommandContext(t.Context(), flock, "--exclusive", "--no-fork", lockPath, "cat") // #nosec G204 -- trusted test command and private lock path.
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	if _, err := fmt.Fprintln(stdin, "locked"); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "locked\n" {
			t.Fatalf("flock holder did not acquire lock: %q", line)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("flock holder did not become ready")
	}
	return cmd
}

type mcpPendingResult struct {
	result *mcp.CallToolResult
	err    error
}

func mcpAsyncCall(ctx context.Context, session *mcp.ClientSession, tool string, args map[string]any) <-chan mcpPendingResult {
	ch := make(chan mcpPendingResult, 1)
	go func() {
		result, err := session.CallTool(ctx, mcpCall(tool, args))
		ch <- mcpPendingResult{result, err}
	}()
	return ch
}

func waitMCPCall(t *testing.T, ch <-chan mcpPendingResult) mcpPendingResult {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(8 * time.Second):
		t.Fatal("MCP call did not finish")
		return mcpPendingResult{}
	}
}

// Observe actual lock waiters via their open file descriptors. A short sleep
// cannot prove that a request reached the store or that it contends on this lock.
func mcpPIDHasLockFD(pid int, lockPath string) bool {
	fds, _ := filepath.Glob(fmt.Sprintf("/proc/%d/fd/*", pid))
	for _, fd := range fds {
		if target, err := os.Readlink(fd); err == nil && target == lockPath {
			return true
		}
	}
	return false
}

func mcpWaitUntil(t *testing.T, description string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mcpWaiterPID(t *testing.T, p *mcpProcess, lockPath string, except int) int {
	t.Helper()
	var pid int
	mcpWaitUntil(t, "MCP child to open the shared lock file", func() bool {
		tasks, _ := filepath.Glob(fmt.Sprintf("/proc/%d/task/*/children", p.cmd.Process.Pid))
		for _, task := range tasks {
			children, _ := os.ReadFile(task) // #nosec G304 -- /proc children file for the test's own server.
			for field := range strings.FieldsSeq(string(children)) {
				child, err := strconv.Atoi(field)
				if err == nil && child != except && mcpPIDHasLockFD(child, lockPath) {
					pid = child
					return true
				}
			}
		}
		return false
	})
	return pid
}

func assertMCPReaped(t *testing.T, pid int) {
	t.Helper()
	mcpWaitUntil(t, fmt.Sprintf("child %d to be reaped", pid), func() bool {
		_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
		return errors.Is(err, os.ErrNotExist)
	})
}

func testMCPProcessLock(t *testing.T, bin string) {
	if runtime.GOOS != "linux" {
		t.Skip("process lock test requires Linux")
	}
	if _, err := os.Stat("/proc/self/task"); err != nil {
		t.Skipf("process lock test requires procfs: %v", err)
	}
	envA, root, tmpA := mcpProcessEnv(t)
	envB, _, tmpB := mcpProcessEnv(t)
	store := filepath.Join(root, "store")
	vault := filepath.Join(root, "vault")
	for _, path := range []string{store, vault} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Only the header is required for MCP preflight. Native initialization
	// cannot run in this test because the library path is deliberately absent.
	mcpWriteFile(t, filepath.Join(store, "chroma.sqlite3"), "SQLite format 3\x00")
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(store, alias); err != nil {
		t.Fatal(err)
	}
	lockPath := mcpLockPath(t, store)
	holder := startMCPFlock(t, lockPath)

	pA := startMCPProcess(t, bin, envA, "mcp", "serve", "--chroma-path="+store, "--max-concurrent-requests=2")
	pB := startMCPProcess(t, bin, envB, "mcp", "serve", "--chroma-path="+alias)
	a, b := pA.connect(t), pB.connect(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	cancelled := mcpAsyncCall(requestCtx, a, "stats", nil)
	cancelledPID := mcpWaiterPID(t, pA, lockPath, 0)
	crashing := mcpAsyncCall(ctx, a, "tags", map[string]any{"list": true})
	crashingPID := mcpWaiterPID(t, pA, lockPath, cancelledPID)
	other := mcpAsyncCall(ctx, b, "stats", nil)
	otherPID := mcpWaiterPID(t, pB, lockPath, 0)

	// Discovery remains available while every slot is occupied. An excess
	// call must complete as a busy error while the OS lock is still held.
	assertMCPDiscovery(t, a)
	busy := waitMCPCall(t, mcpAsyncCall(ctx, a, "stats", nil))
	assertMCPToolError(t, busy.result, busy.err, "busy")
	cancelRequest()
	if got := waitMCPCall(t, cancelled); !errors.Is(got.err, context.Canceled) {
		t.Fatalf("cancelled call did not return the caller's context error: %+v", got)
	}
	assertMCPReaped(t, cancelledPID)
	if !mcpPIDHasLockFD(crashingPID, lockPath) || !mcpPIDHasLockFD(otherPID, lockPath) {
		t.Fatal("cancellation stopped another request")
	}
	crashingProcess, err := os.FindProcess(crashingPID)
	if err != nil {
		t.Fatal(err)
	}
	defer crashingProcess.Release()
	if err := crashingProcess.Kill(); err != nil {
		t.Fatal(err)
	}
	crashed := waitMCPCall(t, crashing)
	assertMCPToolError(t, crashed.result, crashed.err, "")
	assertMCPReaped(t, crashingPID)
	assertMCPDiscovery(t, a)

	// Ordinary CLI writer commands must wait on the same lock as both servers.
	for _, tool := range []string{"ingest", "reset"} {
		cli := exec.CommandContext(ctx, bin, tool, "--format=json", "--vault-path="+vault, "--chroma-path="+alias) // #nosec G204 -- test binary and disposable store/vault paths only.
		cli.Env = envB
		cli.Stdin = strings.NewReader("yes\n")
		if err := cli.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cli.ProcessState == nil {
				_ = cli.Process.Kill()
				_ = cli.Wait()
			}
		})
		mcpWaitUntil(t, tool+" to open the shared lock file", func() bool { return mcpPIDHasLockFD(cli.Process.Pid, lockPath) })
		if err := cli.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if codeErr, ok := errors.AsType[*exec.ExitError](cli.Wait()); !ok || codeErr.ExitCode() != exitError {
			t.Fatalf("ordinary %s lock cancellation: %v", tool, codeErr)
		}
	}

	// A client disconnect must cancel and reap its active children, without
	// affecting another server's child contending on the same canonical store.
	abandoned := mcpAsyncCall(ctx, a, "stats", nil)
	abandonedPID := mcpWaiterPID(t, pA, lockPath, 0)
	pA.shutdown(t, nil)
	assertMCPReaped(t, abandonedPID)
	assertMCPTempEmpty(t, tmpA)
	if got := waitMCPCall(t, abandoned); got.err == nil {
		t.Fatalf("disconnected client received a tool response: %+v", got.result)
	}
	if !mcpPIDHasLockFD(otherPID, lockPath) {
		t.Fatal("client disconnect stopped the other server's request")
	}

	// SIGTERM must also cancel an active lock waiter with stdin still open.
	pSignal := startMCPProcess(t, bin, envA, "mcp", "serve", "--chroma-path="+store)
	signalClient := pSignal.connect(t)
	interrupted := mcpAsyncCall(ctx, signalClient, "stats", nil)
	interruptedPID := mcpWaiterPID(t, pSignal, lockPath, 0)
	pSignal.shutdown(t, syscall.SIGTERM)
	assertMCPReaped(t, interruptedPID)
	assertMCPTempEmpty(t, tmpA)
	_ = waitMCPCall(t, interrupted)

	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if _, ok := errors.AsType[*exec.ExitError](holder.Wait()); !ok {
		t.Fatal("lock holder did not exit by signal")
	}
	got := waitMCPCall(t, other)
	assertMCPToolError(t, got.result, got.err, "")
	assertMCPReaped(t, otherPID)
	assertMCPDiscovery(t, b)
	pB.shutdown(t, syscall.SIGTERM)
	assertMCPTempEmpty(t, tmpB)
}
