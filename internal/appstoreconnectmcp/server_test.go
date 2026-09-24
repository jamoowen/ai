package appstoreconnectmcp

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jamoowen/ai/appstoreconnect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testSpec = `{"openapi":"3.0.1","info":{"title":"test","version":"1"},"security":[{"itc-bearer-token":[]}],"components":{"securitySchemes":{"itc-bearer-token":{"type":"http","scheme":"bearer"}},"schemas":{"App":{"type":"object","properties":{"name":{"type":"string"}}}}},"paths":{"/v1/apps":{"get":{"operationId":"apps_getCollection","responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/App"}}}}}}}}}`

func TestProtocolListsSixToolsAndGatesMutations(t *testing.T) {
	catalog, err := appstoreconnect.LoadCatalog([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	clientTransport := roundTrip(func(*http.Request) (*http.Response, error) {
		h := make(http.Header)
		h.Set("Content-Type", "application/json")
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}, nil
	})
	server := New(Config{Catalog: catalog, Client: &appstoreconnect.Client{Catalog: catalog, Tokens: token("x"), HTTPClient: &http.Client{Transport: clientTransport}}})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), st, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 6 {
		t.Fatalf("got %d tools", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		a := tool.Annotations
		if a == nil {
			t.Fatalf("%s has no annotations", tool.Name)
		}
		switch tool.Name {
		case "asc_search_operations", "asc_describe_operation", "asc_describe_schema":
			if !a.ReadOnlyHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
				t.Fatalf("bad local annotations for %s", tool.Name)
			}
		case "asc_read":
			if !a.ReadOnlyHint || a.OpenWorldHint == nil || !*a.OpenWorldHint {
				t.Fatal("bad read annotations")
			}
		case "asc_write", "asc_delete":
			if a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint || a.IdempotentHint || a.OpenWorldHint == nil || !*a.OpenWorldHint {
				t.Fatalf("bad mutation annotations for %s", tool.Name)
			}
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_search_operations", Arguments: map[string]any{"query": "collection"}})
	if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "apps_getCollection") {
		t.Fatalf("search routing: %#v %v", result, err)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("search structuredContent not an object: %#v", result.StructuredContent)
	}
	operations, ok := structured["operations"].([]any)
	if !ok || len(operations) == 0 {
		t.Fatalf("search structuredContent operations: %#v", structured["operations"])
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_describe_operation", Arguments: map[string]any{"operationId": "apps_getCollection"}})
	if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "operationId") {
		t.Fatalf("describe routing: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_describe_schema", Arguments: map[string]any{"name": "App"}})
	if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "name") {
		t.Fatalf("schema describe routing: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_read", Arguments: map[string]any{"operationId": "apps_getCollection"}})
	if err != nil || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "data") {
		t.Fatalf("read routing: %#v %v", result, err)
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_write", Arguments: map[string]any{"operationId": "apps_getCollection"}})
	if err != nil || !result.IsError {
		t.Fatal("disabled write unexpectedly succeeded")
	}
	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_delete", Arguments: map[string]any{"operationId": "apps_getCollection"}})
	if err != nil || !result.IsError {
		t.Fatal("disabled delete unexpectedly succeeded")
	}
}

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }
