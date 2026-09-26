// Package googleplay provides a constrained Google Play Developer API client.
package googleplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// maxDescriptionBytes limits descriptions to 32 KiB so tool responses stay small.
const maxDescriptionBytes = 32 * 1024

type (
	OperationSummary struct {
		OperationID string `json:"operationId"`
		Method      string `json:"method"`
		Path        string `json:"path"`
		Description string `json:"description,omitempty"`
	}
	parameter struct {
		Type     string   `json:"type"`
		Location string   `json:"location"`
		Required bool     `json:"required"`
		Pattern  string   `json:"pattern,omitempty"`
		Enum     []string `json:"enum,omitempty"`
	}
	method struct {
		ID                  string               `json:"id"`
		Description         string               `json:"description,omitempty"`
		Path                string               `json:"path"`
		HTTPMethod          string               `json:"httpMethod"`
		Parameters          map[string]parameter `json:"parameters"`
		Request             map[string]any       `json:"request"`
		Response            map[string]any       `json:"response"`
		SupportsMediaUpload bool                 `json:"supportsMediaUpload"`
	}
	operation struct {
		OperationSummary
		method method
	}
)

// Catalog is an immutable index over a Google REST Discovery document.
type Catalog struct {
	raw        map[string]any
	schemas    map[string]any
	operations map[string]operation
}

func LoadCatalogFile(path string) (*Catalog, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadCatalog(b)
}

func LoadCatalog(data []byte) (*Catalog, error) {
	if len(data) > maxSpecBytes {
		return nil, fmt.Errorf("Discovery document exceeds 16 MiB")
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decode Discovery JSON: %w", err)
	}
	if raw["kind"] != "discovery#restDescription" || raw["name"] != "androidpublisher" || raw["version"] != "v3" {
		return nil, errors.New("Discovery document must describe androidpublisher v3")
	}
	schemas, _, err := discoveryObject(raw, "schemas")
	if err != nil {
		return nil, err
	}
	c := &Catalog{raw: raw, schemas: schemas, operations: map[string]operation{}}
	if err := c.addResources(raw); err != nil {
		return nil, err
	}
	if len(c.operations) == 0 {
		return nil, fmt.Errorf("Discovery document contains no methods")
	}
	return c, nil
}

func (c *Catalog) addResources(node map[string]any) error {
	methods, hasMethods, err := discoveryObject(node, "methods")
	if err != nil {
		return err
	}
	if hasMethods {
		for _, v := range methods {
			if err := c.addMethod(v); err != nil {
				return err
			}
		}
	}
	resources, hasResources, err := discoveryObject(node, "resources")
	if err != nil {
		return err
	}
	if hasResources {
		for _, v := range resources {
			child, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid resource")
			}
			if err := c.addResources(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// discoveryObject distinguishes an omitted optional Discovery section from a malformed one.
func discoveryObject(node map[string]any, name string) (map[string]any, bool, error) {
	v, ok := node[name]
	if !ok {
		return nil, false, nil
	}
	object, ok := v.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("Discovery %s must be an object", name)
	}
	return object, true, nil
}

func (c *Catalog) addMethod(v any) error {
	raw, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("Discovery method must be an object")
	}
	if request, hasRequest := raw["request"]; hasRequest {
		if _, ok := request.(map[string]any); !ok {
			return fmt.Errorf("Discovery method request must be an object")
		}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var m method
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.ID == "" {
		return fmt.Errorf("Discovery method has no id")
	}
	if _, ok := c.operations[m.ID]; ok {
		return fmt.Errorf("duplicate method id %q", m.ID)
	}
	if !allowedMethod(m.HTTPMethod) {
		return fmt.Errorf("method %s has unsupported HTTP method %q", m.ID, m.HTTPMethod)
	}
	if err := validateDiscoveryPath(m.Path, m.Parameters); err != nil {
		return fmt.Errorf("method %s: %w", m.ID, err)
	}
	c.operations[m.ID] = operation{OperationSummary: OperationSummary{m.ID, m.HTTPMethod, m.Path, m.Description}, method: m}
	return nil
}

func allowedMethod(m string) bool {
	return m == "GET" || m == "POST" || m == "PUT" || m == "PATCH" || m == "DELETE"
}

func validateDiscoveryPath(path string, params map[string]parameter) error {
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "://") || strings.ContainsAny(path, "?#\\") || strings.Contains(path, "..") {
		return fmt.Errorf("unsafe path %q", path)
	}
	remaining := templateRE.ReplaceAllString(path, "")
	if strings.ContainsAny(remaining, "{}") {
		return fmt.Errorf("malformed path template in %q", path)
	}
	used := map[string]bool{}
	for _, match := range templateRE.FindAllStringSubmatch(path, -1) {
		parameter, ok := params[match[2]]
		if !ok || parameter.Location != "path" {
			return fmt.Errorf("path references unknown parameter %q", match[2])
		}
		used[match[2]] = true
	}
	for name, parameter := range params {
		if parameter.Location == "path" && !used[name] {
			return fmt.Errorf("path parameter %q is unused", name)
		}
	}
	return nil
}

var templateRE = regexp.MustCompile(`\{(\+?)([A-Za-z][A-Za-z0-9_]*)(?:=([^{}]+))?\}`)

func (c *Catalog) Search(query, methodFilter string, limit int) []OperationSummary {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query, methodFilter = strings.ToLower(query), strings.ToUpper(methodFilter)
	var out []OperationSummary
	for _, op := range c.operations {
		if methodFilter != "" && methodFilter != op.Method {
			continue
		}
		hay := strings.ToLower(op.OperationID + " " + op.Method + " " + op.Path + " " + op.Description)
		good := true
		for _, term := range strings.Fields(query) {
			if !strings.Contains(hay, term) {
				good = false
			}
		}
		if good {
			out = append(out, op.OperationSummary)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OperationID < out[j].OperationID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (c *Catalog) lookup(id string) (operation, error) {
	op, ok := c.operations[id]
	if !ok {
		return operation{}, fmt.Errorf("unknown operation id %q", id)
	}
	return op, nil
}

func (c *Catalog) Operation(id string) (OperationSummary, error) {
	op, e := c.lookup(id)
	return op.OperationSummary, e
}

func (c *Catalog) Describe(id string) (map[string]any, error) {
	op, e := c.lookup(id)
	if e != nil {
		return nil, e
	}
	return boundedDescription(map[string]any{"method": op.Method, "path": op.Path, "operation": op.method})
}

func (c *Catalog) DescribeSchema(name string) (map[string]any, error) {
	schema, ok := c.schemas[name]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q", name)
	}
	return boundedDescription(map[string]any{"name": name, "schema": schema})
}

func boundedDescription(v map[string]any) (map[string]any, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	if len(b) > maxDescriptionBytes {
		return nil, fmt.Errorf("description is %d bytes; maximum is %d bytes", len(b), maxDescriptionBytes)
	}
	return v, nil
}
