package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpModelSubdir = ".cache/chroma/onnx_models/all-MiniLM-L6-v2/onnx"

type mcpNativeLibraries struct {
	chroma, onnx, tokenizers, modelDir string
}

func mcpLocalArtifact(t *testing.T, envKey string, candidates ...string) string {
	t.Helper()
	if envKey != "" && os.Getenv(envKey) != "" {
		candidates = []string{os.Getenv(envKey)}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				t.Fatal(err)
			}
			return abs
		}
	}
	t.Skipf("local native prerequisite %q not provisioned: tried %v; no downloads attempted", envKey, candidates)
	return ""
}

func mcpNativePrerequisites(t *testing.T) mcpNativeLibraries {
	t.Helper()
	if os.Getenv("NOTEBRAIN_MCP_NATIVE_TEST") != "1" {
		t.Skip("set NOTEBRAIN_MCP_NATIVE_TEST=1 to enable disposable native MCP tests")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("native MCP tests use the shipped Linux amd64 runtime")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	onnxCache := filepath.Join(home, ".cache", "chroma", "shared", "onnxruntime")
	onnxVersions, err := filepath.Glob(filepath.Join(onnxCache, "onnxruntime-linux-x64-*", "lib", "libonnxruntime.so*"))
	if err != nil {
		t.Fatal(err)
	}
	modelDir := filepath.Join(home, mcpModelSubdir)
	mcpLocalArtifact(t, "", filepath.Join(modelDir, "model.onnx"))
	mcpLocalArtifact(t, "", filepath.Join(modelDir, "tokenizer.json"))
	return mcpNativeLibraries{
		chroma:     mcpLocalArtifact(t, "CHROMA_LIB_PATH", filepath.Join(home, ".cache/chroma/local_shim/v0.3.4/linux-amd64/libchroma_shim.so")),
		onnx:       mcpLocalArtifact(t, "CHROMAGO_ONNX_RUNTIME_PATH", append([]string{filepath.Join(onnxCache, "libonnxruntime.so")}, onnxVersions...)...),
		tokenizers: mcpLocalArtifact(t, "TOKENIZERS_LIB_PATH", filepath.Join(home, ".cache/tokenizers/lib/libtokenizers.so")),
		modelDir:   modelDir,
	}
}

func (lib mcpNativeLibraries) env(t *testing.T, withModel bool) (env []string, tmp string) {
	t.Helper()
	env, home, tmp := mcpProcessEnv(t)
	// An explicit preinstalled path bypasses each native library downloader.
	// Deny incidental HTTP(S) fetches as a second guard. Never pass a real
	// HOME, vault, or store to the subprocesses.
	const blockedProxy = "http://127.0.0.1:0"
	env = append(env, "CHROMA_LIB_PATH="+lib.chroma, "CHROMAGO_ONNX_RUNTIME_PATH="+lib.onnx,
		"TOKENIZERS_LIB_PATH="+lib.tokenizers, "HTTP_PROXY="+blockedProxy, "HTTPS_PROXY="+blockedProxy, "NO_PROXY=", "no_proxy=")
	if withModel {
		dst := filepath.Join(home, mcpModelSubdir)
		if err := os.MkdirAll(dst, 0o700); err != nil {
			t.Fatal(err)
		}
		// Link only the read-only artifacts; all cache locks remain in the
		// disposable HOME, not in the user's model cache.
		for _, name := range []string{"model.onnx", "tokenizer.json"} {
			if err := os.Symlink(filepath.Join(lib.modelDir, name), filepath.Join(dst, name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	return env, tmp
}

func mcpNativeCLI(t *testing.T, bin string, env []string, args ...string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cli := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- only the test-built binary and temp fixture data are used.
	cli.Env = env
	var stderr bytes.Buffer
	cli.Stderr = &stderr
	out, err := cli.Output()
	if err != nil {
		t.Fatalf("notebrain %v: %v\nstdout: %s\nstderr: %s", args, err, out, &stderr)
	}
	return out
}

type mcpNativeCase struct {
	name string
	tool string
	args map[string]any
	cli  []string
}

func TestMCPNative(t *testing.T) {
	libs := mcpNativePrerequisites(t)
	bin := buildMCPBinary(t)
	env, tmp := libs.env(t, true)
	root := t.TempDir()
	vault, store := filepath.Join(root, "vault"), filepath.Join(root, "store")
	if err := os.Mkdir(vault, 0o700); err != nil {
		t.Fatal(err)
	}
	mcpWriteFile(t, filepath.Join(vault, "Anchor.md"), "---\ntags: [orchard]\n---\n# Anchor\n\nAn apple orchard needs sunlight, good soil, and careful watering in the summer. [[Companion]]\n\n[Reference](https://example.invalid/orchard)\n")
	mcpWriteFile(t, filepath.Join(vault, "Companion.md"), "---\ntags: [orchard]\n---\n# Companion\n\nA fruit tree can be healthy with seasonal pruning, root care, and a suitable climate. [[Anchor]]\n")
	mcpWriteFile(t, filepath.Join(vault, "Basket.md"), "---\ntags: [harvest]\n---\n# Basket\n\nWoven baskets carry apples and pears from the garden after the harvest season.\n")
	globals := []string{"--vault-path=" + vault, "--chroma-path=" + store}
	mcpNativeCLI(t, bin, env, append([]string{"ingest", "--workers=1", "--min-chunk-words=1"}, globals...)...)

	p := startMCPProcess(t, bin, env, append([]string{"mcp", "serve"}, globals...)...)
	session := p.connect(t)
	assertMCPDiscovery(t, session)
	cases := []mcpNativeCase{
		{"stats", "stats", nil, []string{"stats"}},
		{"get", "get", map[string]any{"note": "anchor"}, []string{"get", "anchor"}},
		{"refs", "refs", map[string]any{"note": "anchor"}, []string{"refs", "anchor"}},
		{"backlinks", "backlinks", map[string]any{"note": "anchor"}, []string{"backlinks", "anchor"}},
		{"connections", "connections", map[string]any{"note": "anchor", "hops": 1}, []string{"connections", "--hops=1", "anchor"}},
		{"search", "search", map[string]any{"queries": []string{"apple orchard"}, "limit": 1}, []string{"search", "--limit=1", "apple orchard"}},
		{"hidden", "hidden", map[string]any{"note": "anchor", "limit": 1}, []string{"hidden", "--limit=1", "anchor"}},
		{"tags_list", "tags", map[string]any{"list": true}, []string{"tags", "--list"}},
		{"boosted", "boosted", map[string]any{"query": "apple orchard", "seed": "anchor", "limit": 1}, []string{"boosted", "--seed=anchor", "--limit=1", "apple orchard"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assertMCPNativeParity(t, bin, env, globals, session, tc) })
	}

	t.Run("without_model_artifacts", func(t *testing.T) {
		noModel, tmp := libs.env(t, false)
		p := startMCPProcess(t, bin, noModel, append([]string{"mcp", "serve"}, globals...)...)
		session := p.connect(t)
		assertMCPNativeParity(t, bin, noModel, globals, session, mcpNativeCase{
			"hidden_deep", "hidden", map[string]any{"note": "anchor", "deep": true, "include_linked": true, "limit": 1},
			[]string{"hidden", "--deep", "--include-linked", "--limit=1", "anchor"},
		})
		assertMCPNativeParity(t, bin, noModel, globals, session, cases[0])
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		boosted := cases[len(cases)-1]
		res, err := session.CallTool(ctx, mcpCall(boosted.tool, boosted.args))
		assertMCPToolError(t, res, err, "model is unavailable")
		assertMCPDiscovery(t, session)
		p.shutdown(t, nil)
		assertMCPTempEmpty(t, tmp)
	})

	t.Run("configuration_frozen_until_restart", func(t *testing.T) {
		noModel, tmp := libs.env(t, false)
		configDir := filepath.Join(root, "config")
		if err := os.Mkdir(configDir, 0700); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(configDir, "notebrain.toml")
		mcpWriteFile(t, file, "list = true\nlimit = 1\nchroma_path = "+strconv.Quote(store)+"\n")
		server := startMCPProcess(t, bin, noModel, "mcp", "serve", "--config="+file)
		client := server.connect(t)
		mcpWriteFile(t, file, "malformed = [")
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		res, err := client.CallTool(ctx, mcpCall("tags", nil))
		data := mcpNativeResultJSON(t, res, err)
		var parsed struct {
			Command string `json:"command"`
			Total   int    `json:"total"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil || parsed.Command != "tags --list" || parsed.Total != 1 {
			t.Fatalf("saved config not applied after rewrite: %s (%v)", data, err)
		}
		res, err = client.CallTool(ctx, mcpCall("tags", map[string]any{"list": false}))
		assertMCPToolError(t, res, err, "requires query")
		server.shutdown(t, nil)
		assertMCPTempEmpty(t, tmp)
	})

	t.Run("successful_concurrent_responses_after_lock_release", func(t *testing.T) {
		// Use the same lock observation as the OS-only test, with a real
		// persistent fixture and three distinct successful response shapes.
		lockPath := mcpLockPath(t, store)
		holder := startMCPFlock(t, lockPath)
		otherEnv, otherTmp := libs.env(t, false)
		other := startMCPProcess(t, bin, otherEnv, append([]string{"mcp", "serve"}, globals...)...)
		otherSession := other.connect(t)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		stats := mcpAsyncCall(ctx, session, "stats", nil)
		statsPID := mcpWaiterPID(t, p, lockPath, 0)
		get := mcpAsyncCall(ctx, session, "get", map[string]any{"note": "anchor"})
		mcpWaiterPID(t, p, lockPath, statsPID)
		refs := mcpAsyncCall(ctx, otherSession, "refs", map[string]any{"note": "anchor"})
		mcpWaiterPID(t, other, lockPath, 0)
		if err := holder.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = holder.Wait()
		for _, pending := range []struct {
			name string
			call <-chan mcpPendingResult
		}{{"stats", stats}, {"get", get}, {"refs", refs}} {
			got := waitMCPCall(t, pending.call)
			data := mcpNativeResultJSON(t, got.result, got.err)
			var envelope struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil || envelope.Command != pending.name {
				t.Errorf("%s response got %s (%v)", pending.name, data, err)
			}
		}
		other.shutdown(t, nil)
		assertMCPTempEmpty(t, otherTmp)
	})
	p.shutdown(t, nil)
	assertMCPTempEmpty(t, tmp)
}

func mcpNativeResultJSON(t *testing.T, res *mcp.CallToolResult, err error) []byte {
	t.Helper()
	if err != nil || res == nil || res.IsError || len(res.Content) != 1 || res.StructuredContent == nil {
		t.Fatalf("MCP retrieval failed: %+v (%v)", res, err)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("retrieval result is not text: %+v", res.Content[0])
	}
	wire, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var structured, asText any
	if err := json.Unmarshal(wire, &structured); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(text.Text), &asText); err != nil || !reflect.DeepEqual(asText, structured) {
		t.Fatalf("text and structured results differ:\ntext: %s\nstructured: %s (%v)", text.Text, wire, err)
	}
	return wire
}

func assertMCPNativeParity(t *testing.T, bin string, env, globals []string, session *mcp.ClientSession, tc mcpNativeCase) {
	t.Helper()
	// Put global options before retrieval arguments, like the MCP subprocess.
	cli := append([]string{tc.cli[0], "--format=json"}, globals...)
	cli = append(cli, tc.cli[1:]...)
	stdout := mcpNativeCLI(t, bin, env, cli...)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	res, err := session.CallTool(ctx, mcpCall(tc.tool, tc.args))
	wire := mcpNativeResultJSON(t, res, err)
	var got, want any
	if err := json.Unmarshal(wire, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stdout, &want); err != nil {
		t.Fatalf("ordinary CLI did not return JSON: %v\n%s", err, stdout)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s result differs from CLI:\nMCP: %s\nCLI: %s", tc.tool, wire, strings.TrimSpace(string(stdout)))
	}
}
