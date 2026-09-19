// Package appstoreconnect provides a constrained App Store Connect API client.
package appstoreconnect

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type OperationSummary struct {
	OperationID string   `json:"operationId"`
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	Tags        []string `json:"tags,omitempty"`
	Deprecated  bool     `json:"deprecated,omitempty"`
}

type operation struct {
	OperationSummary
	op       *openapi3.Operation
	pathItem *openapi3.PathItem
}

// Catalog is an immutable index over an OpenAPI document.
type Catalog struct {
	doc        *openapi3.T
	operations map[string]operation
	raw        map[string]any
}

func LoadCatalogFile(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadCatalog(b)
}

func LoadCatalog(data []byte) (*Catalog, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec: %w", err)
	}
	// Apple's OpenAPI 3.0 document uses the valid 3.0 `nullable` keyword.
	if err := doc.Validate(context.Background(), openapi3.AllowExtraSiblingFields("nullable", "deprecated"), openapi3.DisableExamplesValidation()); err != nil {
		return nil, fmt.Errorf("validate OpenAPI spec: %w", err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode OpenAPI JSON: %w", err)
	}
	c := &Catalog{doc: doc, operations: map[string]operation{}, raw: raw}
	for path, pi := range doc.Paths.Map() {
		for method, op := range map[string]*openapi3.Operation{"GET": pi.Get, "POST": pi.Post, "PUT": pi.Put, "PATCH": pi.Patch, "DELETE": pi.Delete, "HEAD": pi.Head, "OPTIONS": pi.Options, "TRACE": pi.Trace} {
			if op == nil {
				continue
			}
			if op.OperationID == "" {
				return nil, fmt.Errorf("%s %s has no operationId", method, path)
			}
			if _, exists := c.operations[op.OperationID]; exists {
				return nil, fmt.Errorf("duplicate operationId %q", op.OperationID)
			}
			c.operations[op.OperationID] = operation{OperationSummary: OperationSummary{op.OperationID, method, path, append([]string(nil), op.Tags...), op.Deprecated}, op: op, pathItem: pi}
		}
	}
	return c, nil
}

func (c *Catalog) Search(query, method string, limit int) []OperationSummary {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query, method = strings.ToLower(query), strings.ToUpper(method)
	var out []OperationSummary
	for _, item := range c.operations {
		if method != "" && item.Method != method {
			continue
		}
		hay := strings.ToLower(item.OperationID + " " + splitCamel(item.OperationID) + " " + item.Path + " " + strings.Join(item.Tags, " ") + " " + item.op.Description + " " + item.op.Summary)
		for _, p := range item.op.Parameters {
			if p.Value != nil {
				hay += " " + strings.ToLower(p.Value.Name+" "+p.Value.Description)
			}
		}
		matches := query == ""
		if !matches {
			matches = true
			for _, term := range strings.Fields(query) {
				if !strings.Contains(hay, term) {
					matches = false
					break
				}
			}
		}
		if matches {
			out = append(out, item.OperationSummary)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
func splitCamel(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
func (c *Catalog) Operation(id string) (OperationSummary, error) {
	o, ok := c.operations[id]
	if !ok {
		return OperationSummary{}, fmt.Errorf("unknown operationId %q", id)
	}
	return o.OperationSummary, nil
}
func (c *Catalog) lookup(id string) (operation, error) {
	o, ok := c.operations[id]
	if !ok {
		return operation{}, fmt.Errorf("unknown operationId %q", id)
	}
	return o, nil
}

// Describe returns the raw operation object and the component schemas reachable from it.
func (c *Catalog) Describe(id string) (map[string]any, error) {
	o, err := c.lookup(id)
	if err != nil {
		return nil, err
	}
	paths, _ := c.raw["paths"].(map[string]any)
	p, _ := paths[o.Path].(map[string]any)
	opRaw, _ := p[strings.ToLower(o.Method)].(map[string]any)
	result := map[string]any{"operation": opRaw, "schemas": map[string]any{}}
	schemas := result["schemas"].(map[string]any)
	components, _ := c.raw["components"].(map[string]any)
	all, _ := components["schemas"].(map[string]any)
	seen := map[string]bool{}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok && strings.HasPrefix(ref, "#/components/schemas/") {
				n := strings.TrimPrefix(ref, "#/components/schemas/")
				if !seen[n] {
					seen[n] = true
					if s, ok := all[n]; ok {
						schemas[n] = s
						walk(s)
					}
				}
			}
			for _, v := range x {
				walk(v)
			}
		case []any:
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(opRaw)
	return result, nil
}
