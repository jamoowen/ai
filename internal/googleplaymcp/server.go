package googleplaymcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jamoowen/ai/googleplay"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/time/rate"
)

type (
	Config struct {
		Catalog                   *googleplay.Catalog
		Client                    *googleplay.Client
		AllowWrites, AllowDeletes bool
		limiter                   *rate.Limiter
	}
	searchInput struct {
		Query  string `json:"query"`
		Method string `json:"method,omitempty"`
		Limit  int    `json:"limit,omitempty"`
	}
	describeInput struct {
		OperationID string `json:"operationId"`
	}
	schemaInput struct {
		Name string `json:"name"`
	}
	invokeInput = googleplay.Invocation
)

func New(cfg Config) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "googleplay-mcp", Version: "v1"}, &mcp.ServerOptions{Instructions: "Use search, then describe the operation and relevant schemas before invoking it. Never invent operation IDs or schema names. Inspect request schemas before mutations. Google Play does not provide a general API to list every app."})
	limiter := cfg.limiter
	if limiter == nil {
		limiter = rate.NewLimiter(10, 20)
	}
	closed, open, destructive := false, true, true
	local := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &closed}
	remote := &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &open}
	mutation := &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: false, OpenWorldHint: &open}
	mcp.AddTool(s, &mcp.Tool{Name: "gp_search_operations", Description: "Search Google Play Developer API operations", Annotations: local}, func(_ context.Context, _ *mcp.CallToolRequest, in searchInput) (*mcp.CallToolResult, any, error) {
		if !limiter.Allow() {
			return rateLimitResult()
		}
		return jsonResult(map[string]any{"operations": cfg.Catalog.Search(in.Query, in.Method, in.Limit)})
	})
	mcp.AddTool(s, &mcp.Tool{Name: "gp_describe_operation", Description: "Describe a Google Play Developer API operation", Annotations: local}, func(_ context.Context, _ *mcp.CallToolRequest, in describeInput) (*mcp.CallToolResult, any, error) {
		if !limiter.Allow() {
			return rateLimitResult()
		}
		v, e := cfg.Catalog.Describe(in.OperationID)
		if e != nil {
			return nil, nil, e
		}
		return jsonResult(v)
	})
	mcp.AddTool(s, &mcp.Tool{Name: "gp_describe_schema", Description: "Describe one Google Play Discovery schema", Annotations: local}, func(_ context.Context, _ *mcp.CallToolRequest, in schemaInput) (*mcp.CallToolResult, any, error) {
		if !limiter.Allow() {
			return rateLimitResult()
		}
		v, e := cfg.Catalog.DescribeSchema(in.Name)
		if e != nil {
			return nil, nil, e
		}
		return jsonResult(v)
	})
	addInvoke(s, "gp_read", remote, googleplay.ReadOperation, cfg.Client, true, limiter)
	addInvoke(s, "gp_write", mutation, googleplay.WriteOperation, cfg.Client, cfg.AllowWrites, limiter)
	addInvoke(s, "gp_delete", mutation, googleplay.DeleteOperation, cfg.Client, cfg.AllowDeletes, limiter)
	return s
}

func addInvoke(s *mcp.Server, name string, ann *mcp.ToolAnnotations, class googleplay.OperationClass, c *googleplay.Client, enabled bool, limiter *rate.Limiter) {
	mcp.AddTool(s, &mcp.Tool{Name: name, Description: "Invoke a constrained Google Play Developer API operation", Annotations: ann}, func(ctx context.Context, _ *mcp.CallToolRequest, in invokeInput) (*mcp.CallToolResult, any, error) {
		if !limiter.Allow() {
			return rateLimitResult()
		}
		if !enabled {
			if class == googleplay.DeleteOperation {
				return nil, nil, fmt.Errorf("deletes are disabled; set GP_ALLOW_DELETES=true")
			}
			return nil, nil, fmt.Errorf("writes are disabled; set GP_ALLOW_WRITES=true")
		}
		v, e := c.Invoke(ctx, class, in)
		if e != nil {
			if v != nil {
				return invokeErrorResult(v, e)
			}
			return nil, nil, e
		}
		return jsonResult(v)
	})
}

func rateLimitResult() (*mcp.CallToolResult, any, error) {
	tool, structured, err := jsonResult(map[string]any{"error": "Google Play MCP tool rate limit exceeded; retry shortly"})
	if err == nil {
		tool.IsError = true
	}
	return tool, structured, err
}

func invokeErrorResult(r *googleplay.Response, e error) (*mcp.CallToolResult, any, error) {
	result := map[string]any{"error": e.Error(), "response": map[string]any{"status": r.Status, "contentType": r.ContentType, "headers": r.Headers}}
	tool, structured, err := jsonResult(result)
	if err == nil {
		tool.IsError = true
	}
	return tool, structured, err
}

func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, nil, e
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}, StructuredContent: v}, v, nil
}
