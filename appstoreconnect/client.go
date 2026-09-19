package appstoreconnect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

const BaseURL = "https://api.appstoreconnect.apple.com/"

type Invocation struct {
	OperationID    string            `json:"operationId"`
	PathParameters map[string]string `json:"pathParameters,omitempty"`
	Query          map[string]any    `json:"query,omitempty"`
	Body           map[string]any    `json:"body,omitempty"`
}
type TokenSource interface {
	Token(context.Context) (string, error)
}
type Response struct {
	Status      int               `json:"status"`
	ContentType string            `json:"contentType,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        []byte            `json:"body"`
}
type Client struct {
	Catalog          *Catalog
	Tokens           TokenSource
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

func (c *Client) Invoke(ctx context.Context, class string, in Invocation) (*Response, error) {
	if c.Catalog == nil || c.Tokens == nil {
		return nil, errors.New("catalog and token source are required")
	}
	op, err := c.Catalog.lookup(in.OperationID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]string{"read": "GET", "write": "", "delete": "DELETE"}
	if class == "write" && op.Method != "POST" && op.Method != "PATCH" {
		return nil, fmt.Errorf("operation %s is not a write", in.OperationID)
	}
	if class != "write" && allowed[class] != op.Method {
		return nil, fmt.Errorf("operation %s is not permitted by %s", in.OperationID, class)
	}
	path := op.Path
	for _, p := range parameters(op) {
		if p.Value != nil && p.Value.In == "path" {
			v, ok := in.PathParameters[p.Value.Name]
			if !ok {
				return nil, fmt.Errorf("missing path parameter %q", p.Value.Name)
			}
			path = strings.ReplaceAll(path, "{"+p.Value.Name+"}", url.PathEscape(v))
		}
	}
	u, err := url.Parse(BaseURL + strings.TrimPrefix(path, "/"))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	known := map[string]bool{}
	for _, p := range parameters(op) {
		if p.Value == nil || p.Value.In != "query" {
			continue
		}
		known[p.Value.Name] = true
		if v, ok := in.Query[p.Value.Name]; ok {
			addQuery(q, p.Value.Name, v, p.Value.Explode)
		}
	}
	for k := range in.Query {
		if !known[k] {
			return nil, fmt.Errorf("unknown query parameter %q", k)
		}
	}
	u.RawQuery = q.Encode()
	var body io.Reader
	if in.Body != nil {
		b, e := jsonMarshal(in.Body)
		if e != nil {
			return nil, e
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, op.Method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if in.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	tok, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	router, err := gorillamux.NewRouter(c.Catalog.doc)
	if err != nil {
		return nil, err
	}
	route, params, err := router.FindRoute(req)
	if err != nil {
		return nil, fmt.Errorf("find OpenAPI route: %w", err)
	}
	if err := openapi3filter.ValidateRequest(ctx, &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: &openapi3filter.Options{AuthenticationFunc: func(_ context.Context, a *openapi3filter.AuthenticationInput) error {
		if !strings.HasPrefix(req.Header.Get("Authorization"), "Bearer ") {
			return errors.New("missing bearer authorization")
		}
		return nil
	}}}); err != nil {
		return nil, fmt.Errorf("request validation: %w", err)
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("response exceeds %d byte limit", limit)
	}
	out := &Response{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Body: b, Headers: map[string]string{}}
	for _, k := range []string{"X-Request-Id", "X-Rate-Limit-Limit", "X-Rate-Limit-Remaining", "Retry-After"} {
		if v := resp.Header.Get(k); v != "" {
			out.Headers[k] = v
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("App Store Connect returned HTTP %d: %s", resp.StatusCode, string(b))
	}
	return out, nil
}
func addQuery(q url.Values, k string, v any, explode *bool) {
	ex := true
	if explode != nil {
		ex = *explode
	}
	switch x := v.(type) {
	case []any:
		for _, z := range x {
			if ex {
				q.Add(k, fmt.Sprint(z))
			} else {
				q.Set(k, strings.Trim(strings.Join([]string{q.Get(k), fmt.Sprint(z)}, ","), ","))
			}
		}
	case []string:
		for _, z := range x {
			if ex {
				q.Add(k, z)
			} else {
				q.Set(k, strings.Trim(strings.Join([]string{q.Get(k), z}, ","), ","))
			}
		}
	case string:
		q.Set(k, x)
	case bool:
		q.Set(k, strconv.FormatBool(x))
	case float64:
		q.Set(k, strconv.FormatFloat(x, 'f', -1, 64))
	case int:
		q.Set(k, strconv.Itoa(x))
	default:
		q.Set(k, fmt.Sprint(v))
	}
}

func parameters(op operation) []*openapi3.ParameterRef {
	return append(append([]*openapi3.ParameterRef{}, op.pathItem.Parameters...), op.op.Parameters...)
}
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
