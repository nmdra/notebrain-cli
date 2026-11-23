package mcpbridge

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPResult(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		bad        bool
	}{
		{"valid", `{"command":"stats","notes":9007199254740993,"chunks":0,"links":0,"extra":null}`, false},
		{"trailing", `{"command":"stats","notes":0,"chunks":0,"links":0} {}`, true},
		{"array", `[]`, true}, {"null", `null`, true}, {"wrong-command", `{"command":"get","notes":0,"chunks":0,"links":0}`, true}, {"missing", `{"command":"stats"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, e := NewServer("test", 1, nil, executorFunc(func(context.Context, Call) ([]byte, error) { return []byte(tc.body), nil }))
			if e != nil {
				t.Fatal(e)
			}
			cs := connect(t, s)
			r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{}})
			if e != nil {
				t.Fatal(e)
			}
			if r.IsError != tc.bad {
				t.Fatalf("%+v", r)
			}
			if !tc.bad {
				txt := r.Content[0].(*mcp.TextContent).Text
				var v map[string]json.RawMessage
				if e := json.Unmarshal([]byte(txt), &v); e != nil {
					t.Fatal(e)
				}
				if string(v["notes"]) != "9007199254740993" {
					t.Fatal(txt)
				}
			}
		})
	}
}
