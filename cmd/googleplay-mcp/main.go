package main

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/jamoowen/ai/googleplay"
	"github.com/jamoowen/ai/internal/googleplaymcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cat, diagnostics, err := googleplay.LoadCatalogSourceWithDiagnostics(os.Getenv("GP_DISCOVERY_SOURCE"))
	if err != nil {
		fatal(err)
	}
	for _, d := range diagnostics {
		fmt.Fprintln(os.Stderr, "googleplay-mcp:", d)
	}
	tokens, err := googleplay.NewServiceAccountTokenSource(os.Getenv("GP_SERVICE_ACCOUNT_KEY_PATH"))
	if err != nil {
		fatal(err)
	}
	max := int64(1 << 20)
	if raw := os.Getenv("GP_MAX_RESPONSE_BYTES"); raw != "" {
		max, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || max <= 0 {
			fatal(fmt.Errorf("GP_MAX_RESPONSE_BYTES must be a positive integer, got %q", raw))
		}
	}
	server := googleplaymcp.New(googleplaymcp.Config{Catalog: cat, Client: &googleplay.Client{Catalog: cat, Tokens: tokens, MaxResponseBytes: max}, AllowWrites: os.Getenv("GP_ALLOW_WRITES") == "true", AllowDeletes: os.Getenv("GP_ALLOW_DELETES") == "true"})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fatal(err)
	}
}
func fatal(e error) { fmt.Fprintln(os.Stderr, "googleplay-mcp:", e); os.Exit(1) }
