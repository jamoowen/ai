package appstoreconnect

import (
	"context"
	"io"
	"net/http"
	"strings"
)

const fixture = `{"openapi":"3.0.1","info":{"title":"test","version":"1"},"security":[{"itc-bearer-token":[]}],"components":{"securitySchemes":{"itc-bearer-token":{"type":"http","scheme":"bearer"}}},"paths":{"/v1/apps":{"get":{"operationId":"apps_getCollection","responses":{"200":{"description":"ok"}}}},"/v1/apps/{id}":{"parameters":[{"name":"id","description":"path identifier","in":"path","required":true,"schema":{"type":"string"}}],"get":{"operationId":"apps_get","tags":["Apps"],"parameters":[{"name":"filter[name]","description":"operation filter","in":"query","style":"form","explode":false,"schema":{"type":"array","items":{"type":"string"}}},{"name":"include","in":"query","style":"form","explode":true,"schema":{"type":"array","items":{"type":"string"}}},{"name":"mode","in":"query","required":false,"schema":{"type":"string","enum":["one"]}}],"responses":{"200":{"description":"ok"}}},"patch":{"operationId":"apps_patch","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}},"responses":{"200":{"description":"ok"}}},"delete":{"operationId":"apps_delete","responses":{"204":{"description":"ok"}}}}}}`

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(q *http.Request) (*http.Response, error) { return r(q) }

func response(code int, body string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}
