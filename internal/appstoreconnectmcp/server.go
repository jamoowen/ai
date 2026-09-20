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
type invokeInput = appstoreconnect.Invocation

func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "appstoreconnect-mcp", Version: "v1"}, &mcp.ServerOptions{Instructions: "Use search, then describe, then invoke. Never invent operation IDs. Inspect schemas before mutations. Paginate explicitly."})
	closed, open, destructive := false, true, true
	localRead := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closed}
	remoteRead := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &open}
	mutationAnnotations := &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: false, OpenWorldHint: &open}
	mcp.AddTool(s, &mcp.Tool{Name: "asc_search_operations", Description: "Search App Store Connect OpenAPI operations", Annotations: localRead}, func(_ context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		return jsonResult(cfg.Catalog.Search(in.Query, in.Method, in.Limit))
	})
	mcp.AddTool(s, &mcp.Tool{Name: "asc_describe_operation", Description: "Describe an operation and its referenced schemas", Annotations: localRead}, func(_ context.Context, _ *mcp.CallToolRequest, in describeInput) (*mcp.CallToolResult, any, error) {
		v, e := cfg.Catalog.Describe(in.OperationID)
		if e != nil {
			return nil, nil, e
		}
		return jsonResult(v)
	})
	addInvoke(s, "asc_read", remoteRead, appstoreconnect.ReadOperation, cfg.Client, true)
	addInvoke(s, "asc_write", mutationAnnotations, appstoreconnect.WriteOperation, cfg.Client, cfg.AllowWrites)
	addInvoke(s, "asc_delete", mutationAnnotations, appstoreconnect.DeleteOperation, cfg.Client, cfg.AllowDeletes)
	return s
}

func addInvoke(s *mcp.Server, name string, ann *mcp.ToolAnnotations, class appstoreconnect.OperationClass, c *appstoreconnect.Client, enabled bool) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: "Invoke a constrained App Store Connect API operation", Annotations: ann}, func(ctx context.Context, _ *mcp.CallToolRequest, in invokeInput) (*mcp.CallToolResult, any, error) {
		if !enabled {
			if class == appstoreconnect.DeleteOperation {
				return nil, nil, fmt.Errorf("deletes are disabled; set ASC_ALLOW_DELETES=true")
			}
			return nil, nil, fmt.Errorf("writes are disabled; set ASC_ALLOW_WRITES=true")
		}
		v, e := c.Invoke(ctx, class, in)
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
