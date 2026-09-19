package appstoreconnectmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jamoowen/ai/appstoreconnect"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Config struct {
	Catalog                   *appstoreconnect.Catalog
	Client                    *appstoreconnect.Client
	AllowWrites, AllowDeletes bool
}
type searchInput struct {
	Query  string `json:"query"`
	Method string `json:"method,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type describeInput struct {
	OperationID string `json:"operationId"`
}
type invokeInput struct {
	OperationID    string            `json:"operationId"`
	PathParameters map[string]string `json:"pathParameters,omitempty"`
	Query          map[string]any    `json:"query,omitempty"`
	Body           map[string]any    `json:"body,omitempty"`
}

func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "appstoreconnect-mcp", Version: "v1"}, &mcp.ServerOptions{Instructions: "Use search, then describe, then invoke. Never invent operation IDs. Inspect schemas before mutations. Paginate explicitly."})
	ro := &mcp.ToolAnnotations{ReadOnlyHint: true}
	destructive := true
	rw := &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: false}
	mcp.AddTool(s, &mcp.Tool{Name: "asc_search_operations", Description: "Search App Store Connect OpenAPI operations", Annotations: ro}, func(_ context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		return jsonResult(cfg.Catalog.Search(in.Query, in.Method, in.Limit))
	})
	mcp.AddTool(s, &mcp.Tool{Name: "asc_describe_operation", Description: "Describe an operation and its referenced schemas", Annotations: ro}, func(_ context.Context, _ *mcp.CallToolRequest, in describeInput) (*mcp.CallToolResult, any, error) {
		v, e := cfg.Catalog.Describe(in.OperationID)
		if e != nil {
			return nil, nil, e
		}
		return jsonResult(v)
	})
	addInvoke(s, "asc_read", ro, "read", cfg.Client, func() error { return nil })
	addInvoke(s, "asc_write", rw, "write", cfg.Client, func() error {
		if !cfg.AllowWrites {
			return fmt.Errorf("writes are disabled; set ASC_ALLOW_WRITES=true")
		}
		return nil
	})
	addInvoke(s, "asc_delete", rw, "delete", cfg.Client, func() error {
		if !cfg.AllowDeletes {
			return fmt.Errorf("deletes are disabled; set ASC_ALLOW_DELETES=true")
		}
		return nil
	})
	return s
}
func addInvoke(s *mcp.Server, name string, ann *mcp.ToolAnnotations, class string, c *appstoreconnect.Client, policy func() error) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: "Invoke a constrained App Store Connect API operation", Annotations: ann}, func(ctx context.Context, _ *mcp.CallToolRequest, in invokeInput) (*mcp.CallToolResult, any, error) {
		if e := policy(); e != nil {
			return nil, nil, e
		}
		v, e := c.Invoke(ctx, class, appstoreconnect.Invocation{OperationID: in.OperationID, PathParameters: in.PathParameters, Query: in.Query, Body: in.Body})
		if e != nil {
			return nil, nil, e
		}
		return jsonResult(v)
	})
}
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, nil, e
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, v, nil
}
