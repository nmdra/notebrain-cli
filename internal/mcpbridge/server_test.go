package mcpbridge

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPServerConcurrencyAndShutdown(t *testing.T) {
	entered := make(chan struct{}, 2)
	finished := make(chan struct{}, 2)
	s, e := NewServer("test", 2, nil, executorFunc(func(ctx context.Context, _ Call) ([]byte, error) {
		entered <- struct{}{}
		defer func() { finished <- struct{}{} }()
		<-ctx.Done()
		return nil, ctx.Err()
	}))
	if e != nil {
		t.Fatal(e)
	}
	cs := connect(t, s)
	done := make(chan struct{}, 2)
	for range 2 {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{}})
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("SDK handlers did not overlap")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	r, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{}})
	if e != nil || !r.IsError || !strings.Contains(r.Content[0].(*mcp.TextContent).Text, "busy") {
		t.Fatalf("%+v %v", r, e)
	}
	list, e := cs.ListTools(ctx, nil)
	if e != nil || len(list.Tools) != 9 {
		t.Fatal("discovery blocked")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Fatal("executor not joined")
		}
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("request stuck")
		}
	}
}
func TestMCPServerRecoversAfterError(t *testing.T) {
	count := 0
	s, e := NewServer("test", 1, nil, executorFunc(func(context.Context, Call) ([]byte, error) {
		count++
		if count == 1 {
			return nil, fmt.Errorf("secret child stderr")
		}
		return []byte(`{"command":"stats","notes":0,"chunks":0,"links":0}`), nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	cs := connect(t, s)
	for i := range 2 {
		r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "notebrain_stats", Arguments: map[string]any{}})
		if e != nil || r.IsError != (i == 0) {
			t.Fatalf("%+v %v", r, e)
		}
		if i == 0 && strings.Contains(r.Content[0].(*mcp.TextContent).Text, "secret") {
			t.Fatal("leaked stderr")
		}
	}
}
func TestMCPServerConfiguration(t *testing.T) {
	for _, capacity := range []int{-1, 17} {
		if _, e := NewServer("test", capacity, nil, executorFunc(nil)); e == nil {
			t.Fatal("accepted invalid concurrency")
		}
	}
	if _, e := NewServer("test", 4, nil, nil); e == nil {
		t.Fatal("accepted nil executor")
	}
}
