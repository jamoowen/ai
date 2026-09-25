package googleplaymcp

import (
	"context"
	"testing"

	"github.com/jamoowen/ai/googleplay"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }

func TestProtocolListsSixToolsAndGatesMutations(t *testing.T) {
	catalog, err := googleplay.LoadCatalog([]byte(`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"a.get","path":"v3/a","httpMethod":"GET"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Catalog: catalog, Client: &googleplay.Client{Catalog: catalog, Tokens: token("x")}})
	st, ct := mcp.NewInMemoryTransports()
	if _, err = server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) != 6 {
		t.Fatalf("tools=%d err=%v", len(tools.Tools), err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gp_search_operations", Arguments: map[string]any{"query": "a"}})
	if err != nil || result.IsError {
		t.Fatalf("search: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gp_write", Arguments: map[string]any{"operationId": "a.get"}})
	if err != nil || !result.IsError {
		t.Fatalf("write gate: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "gp_delete", Arguments: map[string]any{"operationId": "a.get"}})
	if err != nil || !result.IsError {
		t.Fatalf("delete gate: %#v %v", result, err)
	}
}

func TestToolRateLimitReturnsMCPError(t *testing.T) {
	catalog, err := googleplay.LoadCatalog([]byte(`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"a.get","path":"v3/a","httpMethod":"GET"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Catalog: catalog,
		Client:  &googleplay.Client{Catalog: catalog, Tokens: token("x")},
		limiter: rate.NewLimiter(0, 1),
	})
	st, ct := mcp.NewInMemoryTransports()
	if _, err = server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	params := &mcp.CallToolParams{Name: "gp_search_operations", Arguments: map[string]any{"query": "a"}}
	result, err := session.CallTool(context.Background(), params)
	if err != nil || result.IsError {
		t.Fatalf("first call: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), params)
	if err != nil || !result.IsError {
		t.Fatalf("rate-limited call: %#v %v", result, err)
	}
}
