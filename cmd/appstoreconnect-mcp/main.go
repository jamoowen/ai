package main

import (
	"context"
	"fmt"
	"github.com/jamoowen/ai/appstoreconnect"
	"github.com/jamoowen/ai/internal/appstoreconnectmcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"strconv"
)

func main() {
	source := os.Getenv("ASC_OPENAPI_SOURCE")
	cat, err := appstoreconnect.LoadCatalogSource(source)
	if err != nil {
		fatal(err)
	}
	token, err := appstoreconnect.NewES256TokenSource(os.Getenv("ASC_KEY_ID"), os.Getenv("ASC_ISSUER_ID"), os.Getenv("ASC_PRIVATE_KEY_PATH"), 0, nil)
	if err != nil {
		fatal(err)
	}
	max := int64(1 << 20)
	if raw := os.Getenv("ASC_MAX_RESPONSE_BYTES"); raw != "" {
		max, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || max <= 0 {
			fatal(fmt.Errorf("ASC_MAX_RESPONSE_BYTES must be positive"))
		}
	}
	server := appstoreconnectmcp.New(appstoreconnectmcp.Config{Catalog: cat, Client: &appstoreconnect.Client{Catalog: cat, Tokens: token, MaxResponseBytes: max}, AllowWrites: os.Getenv("ASC_ALLOW_WRITES") == "true", AllowDeletes: os.Getenv("ASC_ALLOW_DELETES") == "true"})
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "appstoreconnect-mcp:", err); os.Exit(1) }
