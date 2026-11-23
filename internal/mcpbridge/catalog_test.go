package mcpbridge

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type executorFunc func(context.Context, Call) ([]byte, error)

func (f executorFunc) Execute(c context.Context, call Call) ([]byte, error) { return f(c, call) }

func connect(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	a, b := mcp.NewInMemoryTransports()
	ss, e := s.SDKServer().Connect(context.Background(), a, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { ss.Close(); s.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, e := client.Connect(context.Background(), b, nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}
func TestMCPCatalog(t *testing.T) {
	s, e := NewServer("test", 4, nil, executorFunc(func(context.Context, Call) ([]byte, error) {
		return []byte(`{"command":"stats","notes":0,"chunks":0,"links":0}`), nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	cs := connect(t, s)
	list, e := cs.ListTools(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]bool{"search": true, "get": true, "refs": true, "backlinks": true, "connections": true, "hidden": true, "tags": true, "boosted": true, "stats": true}
	if len(list.Tools) != 9 {
		t.Fatalf("tools=%d", len(list.Tools))
	}
	for _, tool := range list.Tools {
		if !want[tool.Name[len("notebrain_"):]] {
			t.Fatal(tool.Name)
		}
		if tool.OutputSchema != nil {
			t.Fatal("unexpected output schema")
		}
	}
	good, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{}})
	if e != nil || good.IsError {
		t.Fatalf("result=%+v err=%v", good, e)
	}
	bad, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{"config": "bad"}})
	if e == nil && !bad.IsError {
		t.Fatal("unknown property accepted")
	}
}
