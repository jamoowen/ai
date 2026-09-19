package appstoreconnect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

const BaseURL = "https://api.appstoreconnect.apple.com/"

type OperationClass string

const (
	ReadOperation   OperationClass = "read"
	WriteOperation  OperationClass = "write"
	DeleteOperation OperationClass = "delete"
)

type Invocation struct {
	OperationID    string            `json:"operationId"`
	PathParameters map[string]string `json:"pathParameters,omitempty"`
	Query          map[string]any    `json:"query,omitempty"`
	Body           map[string]any    `json:"body,omitempty"`
}
type TokenSource interface {
	Token(context.Context) (string, error)
}

// Response contains only JSON values or text strings, never binary data.
type Response struct {
	Status      int               `json:"status"`
	ContentType string            `json:"contentType,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        any               `json:"body,omitempty"`
}
type Client struct {
	Catalog          *Catalog
	Tokens           TokenSource
	HTTPClient       *http.Client
	MaxResponseBytes int64
}

func (c *Client) Invoke(ctx context.Context, class OperationClass, in Invocation) (*Response, error) {
	if c.Catalog == nil || c.Tokens == nil {
		return nil, errors.New("catalog and token source are required")
	}
	op, err := c.Catalog.lookup(in.OperationID)
	if err != nil {
		return nil, err
	}
	if !classAllows(class, op.Method) {
		return nil, fmt.Errorf("operation %s is not permitted by %s", in.OperationID, class)
	}
	path, err := buildPath(op, in.PathParameters)
	if err != nil {
		return nil, err
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
		b, e := json.Marshal(in.Body)
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
	if err := openapi3filter.ValidateRequest(ctx, &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: &openapi3filter.Options{AuthenticationFunc: authenticateBearer}}); err != nil {
		return nil, fmt.Errorf("request validation: %w", err)
	}
	resp, err := c.httpClient().Do(req)
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
	out := &Response{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Headers: safeHeaders(resp.Header)}
	parsed, err := parseResponseBody(out.ContentType, b)
	if err != nil {
		return out, err
	}
	out.Body = parsed
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("App Store Connect returned HTTP %d: %s", resp.StatusCode, boundedDetail(parsed))
	}
	return out, nil
}
func classAllows(class OperationClass, method string) bool {
	switch class {
	case ReadOperation:
		return method == "GET"
	case WriteOperation:
		return method == "POST" || method == "PATCH"
	case DeleteOperation:
		return method == "DELETE"
	default:
		return false
	}
}
func buildPath(op operation, provided map[string]string) (string, error) {
	path := op.Path
	known := map[string]bool{}
	for _, p := range parameters(op) {
		if p.Value == nil || p.Value.In != "path" {
			continue
		}
		known[p.Value.Name] = true
		v, ok := provided[p.Value.Name]
		if !ok {
			return "", fmt.Errorf("missing path parameter %q", p.Value.Name)
		}
		path = strings.ReplaceAll(path, "{"+p.Value.Name+"}", url.PathEscape(v))
	}
	for k := range provided {
		if !known[k] {
			return "", fmt.Errorf("unknown path parameter %q", k)
		}
	}
	return path, nil
}
func authenticateBearer(_ context.Context, in *openapi3filter.AuthenticationInput) error {
	if in.SecuritySchemeName != "itc-bearer-token" {
		return fmt.Errorf("unexpected security scheme %q", in.SecuritySchemeName)
	}
	auth := in.RequestValidationInput.Request.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == "" {
		return errors.New("missing bearer authorization")
	}
	return nil
}
func (c *Client) httpClient() *http.Client {
	base := c.HTTPClient
	if base == nil {
		base = http.DefaultClient
	}
	clone := *base
	prior := clone.CheckRedirect
	clone.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != "api.appstoreconnect.apple.com" {
			return errors.New("redirect leaves App Store Connect host")
		}
		if prior != nil {
			return prior(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &clone
}
func parseResponseBody(contentType string, b []byte) (any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("invalid response content type: %w", err)
	}
	if media == "application/json" || strings.HasSuffix(media, "+json") {
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, fmt.Errorf("invalid JSON response: %w", err)
		}
		return v, nil
	}
	if strings.HasPrefix(media, "text/") {
		return string(b), nil
	}
	return nil, fmt.Errorf("unsupported binary response content type %q", contentType)
}
func boundedDetail(v any) string { b, _ := json.Marshal(v); return string(b) }
func safeHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"X-Request-Id", "X-Rate-Limit-Limit", "X-Rate-Limit-Remaining", "Retry-After"} {
		if v := h.Get(k); v != "" {
			out[k] = v
		}
	}
	return out
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
