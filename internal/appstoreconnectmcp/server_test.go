package appstoreconnectmcp

import (
	"context"
	"testing"

	"github.com/jamoowen/ai/appstoreconnect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const testSpec = `{"openapi":"3.0.1","info":{"title":"test","version":"1"},"paths":{"/v1/apps":{"get":{"operationId":"apps_getCollection","responses":{"200":{"description":"ok"}}}}}}`

func TestProtocolListsFiveToolsAndGatesMutations(t *testing.T) {
	catalog, err := appstoreconnect.LoadCatalog([]byte(testSpec))
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Catalog: catalog, Client: &appstoreconnect.Client{Catalog: catalog, Tokens: token("x")}})
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
	if len(tools.Tools) != 5 {
		t.Fatalf("got %d tools", len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if tool.Name == "asc_write" && (tool.Annotations == nil || tool.Annotations.ReadOnlyHint) {
			t.Fatal("write annotations are not conservative")
		}
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "asc_write", Arguments: map[string]any{"operationId": "apps_getCollection"}})
	if err != nil || !result.IsError {
		t.Fatal("disabled write unexpectedly succeeded")
	}
}

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }
