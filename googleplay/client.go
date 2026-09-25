package googleplay

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
	"regexp"
	"strconv"
	"strings"
)

const BaseURL = "https://androidpublisher.googleapis.com/"

type OperationClass string

const (
	ReadOperation   OperationClass = "read"
	WriteOperation  OperationClass = "write"
	DeleteOperation OperationClass = "delete"
)

type (
	Invocation struct {
		OperationID    string            `json:"operationId"`
		PathParameters map[string]string `json:"pathParameters,omitempty"`
		Query          map[string]any    `json:"query,omitempty"`
		Body           map[string]any    `json:"body,omitempty"`
	}
	Response struct {
		Status      int               `json:"status"`
		ContentType string            `json:"contentType,omitempty"`
		Headers     map[string]string `json:"headers,omitempty"`
		Body        any               `json:"body,omitempty"`
	}
	Client struct {
		Catalog          *Catalog
		Tokens           TokenSource
		HTTPClient       *http.Client
		MaxResponseBytes int64
	}
)

func (c *Client) Invoke(ctx context.Context, class OperationClass, in Invocation) (*Response, error) {
	if c.Catalog == nil || c.Tokens == nil {
		return nil, errors.New("catalog and token source are required")
	}
	operation, err := c.Catalog.lookup(in.OperationID)
	if err != nil {
		return nil, err
	}
	if !classAllows(class, operation.Method) {
		return nil, fmt.Errorf("operation %s is not permitted by %s", in.OperationID, class)
	}
	if operation.method.SupportsMediaUpload {
		return nil, fmt.Errorf("operation %s supports media upload, which is unsupported", in.OperationID)
	}
	if in.Body != nil && operation.method.Request == nil {
		return nil, fmt.Errorf("operation %s does not accept a request body", in.OperationID)
	}
	if in.Body == nil && operation.method.Request != nil {
		return nil, fmt.Errorf("operation %s requires a JSON request body", in.OperationID)
	}
	if err := validateBody(c.Catalog, operation, in.Body); err != nil {
		return nil, err
	}
	path, err := buildPath(operation, in.PathParameters)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(BaseURL + path)
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	for name, value := range in.Query {
		parameter, ok := operation.method.Parameters[name]
		if !ok || parameter.Location != "query" {
			return nil, fmt.Errorf("unknown query parameter %q", name)
		}
		if err := addQuery(query, name, value, parameter); err != nil {
			return nil, err
		}
	}
	for name, parameter := range operation.method.Parameters {
		if parameter.Location == "query" && parameter.Required {
			if _, ok := in.Query[name]; !ok {
				return nil, fmt.Errorf("missing query parameter %q", name)
			}
		}
	}
	endpoint.RawQuery = query.Encode()
	var body io.Reader
	if in.Body != nil {
		encodedBody, err := json.Marshal(in.Body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		body = bytes.NewReader(encodedBody)
	}
	request, err := http.NewRequestWithContext(ctx, operation.Method, endpoint.String(), body)
	if err != nil {
		return nil, err
	}
	if in.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("empty OAuth access token")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	httpResponse, err := c.httpClient().Do(request)
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()
	response := &Response{Status: httpResponse.StatusCode, ContentType: httpResponse.Header.Get("Content-Type"), Headers: safeHeaders(httpResponse.Header)}
	limit := c.MaxResponseBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(httpResponse.Body, limit+1))
	if err != nil {
		return response, err
	}
	if int64(len(bodyBytes)) > limit {
		return response, fmt.Errorf("response exceeds %d byte limit", limit)
	}
	parsed, err := parseResponseBody(response.ContentType, bodyBytes)
	if err == nil {
		response.Body = parsed
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		if err != nil {
			return response, fmt.Errorf("Google Play Developer API returned HTTP %d: %w", httpResponse.StatusCode, err)
		}
		return response, fmt.Errorf("Google Play Developer API returned HTTP %d: %s", httpResponse.StatusCode, boundedDetail(parsed))
	}
	if err != nil {
		return response, err
	}
	return response, nil
}

func classAllows(c OperationClass, m string) bool {
	switch c {
	case ReadOperation:
		return m == "GET"
	case WriteOperation:
		return m == "POST" || m == "PUT" || m == "PATCH"
	case DeleteOperation:
		return m == "DELETE"
	}
	return false
}

func buildPath(op operation, provided map[string]string) (string, error) {
	path := op.Path
	for _, match := range templateRE.FindAllStringSubmatch(op.Path, -1) {
		plus, name, templatePattern := match[1] == "+", match[2], match[3]
		p := op.method.Parameters[name]
		v, ok := provided[name]
		if !ok {
			return "", fmt.Errorf("missing path parameter %q", name)
		}
		if e := validatePathValue(name, v, p, plus || templatePattern != "", templatePattern); e != nil {
			return "", e
		}
		escaped := url.PathEscape(v)
		if plus {
			escaped = strings.Join(escapePathSegments(v), "/")
		}
		path = strings.Replace(path, match[0], escaped, 1)
	}
	for k := range provided {
		p, ok := op.method.Parameters[k]
		if !ok || p.Location != "path" {
			return "", fmt.Errorf("unknown path parameter %q", k)
		}
	}
	return path, nil
}

func escapePathSegments(v string) []string {
	parts := strings.Split(v, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return parts
}

func validatePathValue(name, v string, p parameter, reserved bool, templatePattern string) error {
	if v == "" || strings.ContainsAny(v, "?#\\\x00\r\n") || strings.Contains(v, "..") {
		return fmt.Errorf("invalid path parameter %q", name)
	}
	if !reserved && strings.Contains(v, "/") {
		return fmt.Errorf("path parameter %q may not contain /", name)
	}
	if p.Pattern != "" {
		r, e := regexp.Compile(p.Pattern)
		if e != nil {
			return fmt.Errorf("invalid Discovery pattern for %q", name)
		}
		if !r.MatchString(v) {
			return fmt.Errorf("path parameter %q does not match its required pattern", name)
		}
	}
	if templatePattern != "" && !matchesTemplatePattern(v, templatePattern) {
		return fmt.Errorf("path parameter %q does not match its template", name)
	}
	return nil
}

func matchesTemplatePattern(value, pattern string) bool {
	valueParts, patternParts := strings.Split(value, "/"), strings.Split(pattern, "/")
	if len(valueParts) != len(patternParts) {
		return false
	}
	for i, segment := range patternParts {
		if segment != "*" && segment != valueParts[i] {
			return false
		}
	}
	return true
}

func validateBody(c *Catalog, op operation, body map[string]any) error {
	if body == nil {
		return nil
	}
	ref, _ := op.method.Request["$ref"].(string)
	if ref == "" {
		return nil
	}
	raw, ok := c.schemas[ref].(map[string]any)
	if !ok {
		return nil
	}
	if typ, _ := raw["type"].(string); typ != "" && typ != "object" {
		return fmt.Errorf("request schema %s is not an object", ref)
	}
	props, _ := raw["properties"].(map[string]any)
	for key := range body {
		if _, ok := props[key]; !ok {
			return fmt.Errorf("unknown request body field %q", key)
		}
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	return clientWithRedirectPolicy(c.HTTPClient, func(r *http.Request) bool {
		return r.URL.Scheme == "https" && r.URL.Host == "androidpublisher.googleapis.com"
	}, "redirect leaves Google Play Developer API host")
}

func addQuery(q url.Values, k string, v any, p parameter) error {
	if v == nil {
		return nil
	}
	var values []string
	switch x := v.(type) {
	case []any:
		for _, z := range x {
			s, e := scalar(z)
			if e != nil {
				return fmt.Errorf("invalid query parameter %q: %w", k, e)
			}
			values = append(values, s)
		}
	default:
		s, e := scalar(x)
		if e != nil {
			return fmt.Errorf("invalid query parameter %q: %w", k, e)
		}
		values = []string{s}
	}
	for _, s := range values {
		if p.Type == "boolean" && s != "true" && s != "false" {
			return fmt.Errorf("invalid query parameter %q", k)
		}
		if p.Type == "integer" {
			if _, e := strconv.ParseInt(s, 10, 64); e != nil {
				return fmt.Errorf("invalid query parameter %q", k)
			}
		}
		if len(p.Enum) > 0 && !contains(p.Enum, s) {
			return fmt.Errorf("invalid query parameter %q", k)
		}
		q.Add(k, s)
	}
	return nil
}

func scalar(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(x), nil
	default:
		return "", fmt.Errorf("unsupported value type %T", v)
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func parseResponseBody(ct string, b []byte) (any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	media, _, e := mime.ParseMediaType(ct)
	if e != nil {
		return nil, fmt.Errorf("invalid response content type: %w", e)
	}
	if media == "application/json" || strings.HasSuffix(media, "+json") {
		var v any
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, fmt.Errorf("invalid JSON response: %w", e)
		}
		return v, nil
	}
	if strings.HasPrefix(media, "text/") {
		return string(b), nil
	}
	return nil, fmt.Errorf("unsupported binary response content type %q", ct)
}

func boundedDetail(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return "unable to marshal response detail"
	}
	const max = 4 << 10
	const marker = "... (truncated)"
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max-len(marker)]) + marker
}

func safeHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for _, k := range []string{"X-Request-Id", "X-Google-Request-Id", "Retry-After"} {
		if v := h.Get(k); v != "" {
			out[k] = v
		}
	}
	return out
}
