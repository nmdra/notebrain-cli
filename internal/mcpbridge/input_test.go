package mcpbridge

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func invoke(t *testing.T, command string, args map[string]any, defaults map[string]string) (*mcp.CallToolResult, *Call) {
	t.Helper()
	var call *Call
	s, e := NewServer("test", 4, defaults, executorFunc(func(_ context.Context, c Call) ([]byte, error) {
		call = &c
		effective := clone(defaults)
		maps.Copy(effective, c.Overrides)
		name := expectedCommand(c, effective)
		var body string
		switch c.Command {
		case "get":
			body = `"query":"n","note":{}`
		case "refs":
			body = `"note_slug":"n","title":"N","total":0,"refs":null`
		case "stats":
			body = `"notes":0,"chunks":0,"links":0`
		default:
			if name == "tags --list" {
				body = `"total":0,"tags":[]`
			} else {
				body = `"query":"q","total":0,"results":[]`
			}
		}
		b, _ := json.Marshal(name)
		return []byte(`{"command":` + string(b) + `,` + body + `}`), nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	cs := connect(t, s)
	r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_" + command, Arguments: args})
	if e != nil {
		return &mcp.CallToolResult{IsError: true}, call
	}
	return r, call
}
func TestMCPInputMappings(t *testing.T) {
	for _, tc := range []struct {
		command string
		input   map[string]any
		args    []string
		flags   map[string]string
	}{
		{"search", map[string]any{"queries": []string{"-x", "世界"}, "tag": "t", "section": "s", "has_tasks": false, "has_code": true, "with_pdf": false, "exclude_notes": []string{`a,b`, `c\d`}, "limit": 0, "top_k": 1, "include_text": true, "context_window": 0, "min_score": -3.5, "group_by_note": false}, []string{"-x", "世界"}, map[string]string{"tag": "t", "section": "s", "has-tasks": "false", "has-code": "true", "with-pdf": "false", "exclude-notes": `["a,b","c\\d"]`, "limit": "0", "top-k": "1", "include-text": "true", "context-window": "0", "min-score": "-3.5", "group-by-note": "false"}},
		{"get", map[string]any{"note": "-n", "meta": true, "head": 0}, []string{"-n"}, map[string]string{"meta": "true", "head": "0"}},
		{"refs", map[string]any{"note": "n", "only_images": true, "only_pdf": false, "only_other": true, "only_external_links": true, "include_missing": true}, []string{"n"}, map[string]string{"only-images": "true", "only-pdf": "false", "only-other": "true", "only-external-links": "true", "include-missing": "true"}},
		{"backlinks", map[string]any{"note": "n"}, []string{"n"}, map[string]string{}},
		{"connections", map[string]any{"note": "n", "hops": 3}, []string{"n"}, map[string]string{"hops": "3"}},
		{"hidden", map[string]any{"note": "n", "deep": true, "include_linked": true, "candidate_chunks": 10, "limit": 100}, []string{"n"}, map[string]string{"deep": "true", "include-linked": "true", "candidate-chunks": "10", "limit": "100"}},
		{"tags", map[string]any{"query": "n", "list": false, "shared": false, "for_note": true, "children": true, "min_shared": 100, "limit": 0}, []string{"n"}, map[string]string{"list": "false", "shared": "false", "for-note": "true", "children": "true", "min-shared": "100", "limit": "0"}},
		{"boosted", map[string]any{"query": "q", "seed": "n", "boost": 0, "with_pdf": true, "limit": 1}, []string{"q"}, map[string]string{"seed": "n", "boost": "0", "with-pdf": "true", "limit": "1"}},
		{"stats", map[string]any{}, nil, map[string]string{}},
	} {
		t.Run(tc.command, func(t *testing.T) {
			r, c := invoke(t, tc.command, tc.input, nil)
			if r.IsError || c == nil {
				t.Fatalf("%+v", r)
			}
			if !reflect.DeepEqual(c.Args, tc.args) || !reflect.DeepEqual(c.Overrides, tc.flags) {
				t.Fatalf("call=%+v", c)
			}
		})
	}
}
func TestMCPInputBounds(t *testing.T) {
	for _, tc := range []struct {
		command, key string
		lo, hi       int
	}{
		{"search", "limit", 0, 100}, {"search", "top_k", 1, 3}, {"connections", "hops", 1, 3}, {"hidden", "candidate_chunks", 1, 10}, {"hidden", "context_window", 0, 5}, {"get", "head", 0, 100}, {"tags", "min_shared", 1, 100}, {"boosted", "boost", 0, 10},
	} {
		for _, v := range []int{tc.lo - 1, tc.lo, tc.hi, tc.hi + 1} {
			t.Run(tc.key+"="+string(rune(v+200)), func(t *testing.T) {
				args := map[string]any{tc.key: v}
				switch tc.command {
				case "search":
					args["queries"] = []string{"q"}
				case "tags":
					args["query"] = "t"
				case "boosted":
					args["query"] = "q"
					args["seed"] = "s"
				default:
					args["note"] = "n"
				}
				r, _ := invoke(t, tc.command, args, nil)
				if r.IsError != (v < tc.lo || v > tc.hi) {
					t.Fatalf("v=%d result=%+v", v, r)
				}
				delete(args, tc.key)
				r, _ = invoke(t, tc.command, args, map[string]string{strings.ReplaceAll(tc.key, "_", "-"): jsonNumber(v)})
				if r.IsError != (v < tc.lo || v > tc.hi) {
					t.Fatalf("configured v=%d result=%+v", v, r)
				}
			})
		}
	}
}
func jsonNumber(v int) string { b, _ := json.Marshal(v); return string(b) }
func TestMCPInputModesAndLimits(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		input         map[string]any
		defaults      map[string]string
		bad           bool
	}{
		{"tag-only", "search", map[string]any{}, map[string]string{"tag": "t"}, false},
		{"empty-search", "search", map[string]any{"queries": []string{" "}}, nil, true},
		{"empty-note", "get", map[string]any{"note": " "}, nil, true},
		{"missing-note", "refs", map[string]any{}, nil, true},
		{"missing-seed", "boosted", map[string]any{"query": "q"}, nil, true},
		{"list-priority", "tags", map[string]any{}, map[string]string{"list": "true", "shared": "true"}, false},
		{"tags-query", "tags", map[string]any{}, nil, true},
		{"explicit-false", "tags", map[string]any{"list": false}, map[string]string{"list": "true"}, true},
		{"clear-exclusions", "search", map[string]any{"exclude_notes": []string{}}, map[string]string{"tag": "t", "exclude-notes": `["old"]`}, false},
		{"oversized-string", "get", map[string]any{"note": strings.Repeat("x", 16385)}, nil, true},
		{"too-many-queries", "search", map[string]any{"queries": make([]string, 33)}, nil, true},
		{"too-many-exclusions", "search", map[string]any{"tag": "t", "exclude_notes": make([]string, 257)}, nil, true},
		{"total-bytes", "search", map[string]any{"queries": []string{strings.Repeat("x", 16384), strings.Repeat("y", 16384), strings.Repeat("z", 16384), strings.Repeat("a", 16384), "b"}}, nil, true},
		{"bad-config-score", "search", map[string]any{"tag": "t"}, map[string]string{"min-score": "NaN"}, true},
		{"override-bad-config", "search", map[string]any{"tag": "t", "limit": 1}, map[string]string{"limit": "101"}, false},
		{"unknown", "search", map[string]any{"tag": "t", "format": "text"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := clone(tc.defaults)
			r, c := invoke(t, tc.command, tc.input, tc.defaults)
			if r.IsError != tc.bad {
				t.Fatalf("%+v", r)
			}
			if tc.name == "clear-exclusions" && (c == nil || c.Overrides["exclude-notes"] != "[]") {
				t.Fatal(c)
			}
			for k, v := range before {
				if tc.defaults[k] != v {
					t.Fatal("mutated defaults")
				}
			}
		})
	}
}
