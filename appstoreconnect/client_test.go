package appstoreconnect

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientBuildsRequestsAndGatesMethods(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.appstoreconnect.apple.com" || r.URL.EscapedPath() != "/v1/apps/a%2Fb" || r.URL.Query().Get("filter[name]") != "a,b" || strings.Join(r.URL.Query()["include"], ",") != "a,b" {
			t.Fatalf("bad request %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("missing auth")
		}
		return response(200, "{}"), nil
	})
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: rt}}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "a/b"}, Query: map[string]any{"filter[name]": []string{"a", "b"}, "include": []string{"a", "b"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_patch"}); err == nil {
		t.Fatal("write accepted as read")
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "apps_patch", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{}}); err == nil {
		t.Fatal("invalid body reached transport")
	}
}

func TestClientRejectsUnknownPathAndRedirects(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Location", "https://example.com/nope")
		return &http.Response{StatusCode: http.StatusFound, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x", "extra": "no"}}); err == nil {
		t.Fatal("unknown path parameter was accepted")
	}
	if calls != 0 {
		t.Fatal("invalid request reached transport")
	}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}}); err == nil {
		t.Fatal("cross-host redirect was accepted")
	}
	if calls != 1 {
		t.Fatalf("got %d requests; redirect should not reach another host", calls)
	}
}

func TestClientRedirectLimitIsPreserved(t *testing.T) {
	c := &Client{}
	req, _ := http.NewRequest("GET", BaseURL, nil)
	via := make([]*http.Request, 10)
	if err := c.httpClient().CheckRedirect(req, via); err == nil {
		t.Fatal("default redirect limit removed")
	}
	called := false
	c.HTTPClient = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		called = true
		return http.ErrUseLastResponse
	}}
	if err := c.httpClient().CheckRedirect(req, nil); err != http.ErrUseLastResponse || !called {
		t.Fatalf("custom redirect policy was not preserved: %v, called=%t", err, called)
	}
}

func TestClientRejectsInvalidInputsBeforeTransportAndGatesMethods(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return response(200, "{}"), nil })}}
	cases := []Invocation{
		{OperationID: "apps_get"},
		{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}, Query: map[string]any{"unknown": "x"}},
		{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}, Query: map[string]any{"mode": "bad"}},
		{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{}},
	}
	for _, in := range cases {
		if _, err := client.Invoke(context.Background(), ReadOperation, in); err == nil {
			t.Fatalf("invalid input accepted: %#v", in)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid inputs reached transport: %d", calls)
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "apps_patch", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{}}); err == nil {
		t.Fatal("invalid body accepted")
	}
	if calls != 0 {
		t.Fatal("invalid body reached transport")
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}}); err == nil {
		t.Fatal("GET allowed as write")
	}
	if _, err := client.Invoke(context.Background(), DeleteOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}}); err == nil {
		t.Fatal("GET allowed as delete")
	}
	if _, err := client.Invoke(context.Background(), DeleteOperation, Invocation{OperationID: "apps_delete", PathParameters: map[string]string{"id": "x"}}); err != nil {
		t.Fatalf("DELETE routing: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected only successful delete transport, got %d", calls)
	}
	if _, err := client.Invoke(context.Background(), DeleteOperation, Invocation{OperationID: "apps_delete", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{}}); err == nil {
		t.Fatal("DELETE body accepted")
	}
	if calls != 1 {
		t.Fatal("DELETE body reached transport")
	}
	empty := &Client{Catalog: c, Tokens: token(""), HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return response(200, "{}"), nil })}}
	if _, err := empty.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}}); err == nil {
		t.Fatal("empty bearer accepted")
	}
	if calls != 1 {
		t.Fatal("empty bearer reached transport")
	}
}

func TestClientPermitsValidPostAndPatchWrites(t *testing.T) {
	post := `"/v1/apps-create":{"post":{"operationId":"apps_create","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}},"responses":{"200":{"description":"ok"}}}},`
	c, err := LoadCatalog([]byte(strings.Replace(fixture, `"/v1/apps/{id}"`, post+`"/v1/apps/{id}"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	var methods []string
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		methods = append(methods, r.Method)
		return response(200, "{}"), nil
	})}}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "apps_create", Body: map[string]any{"name": "new"}}); err != nil {
		t.Fatalf("POST write: %v", err)
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "apps_patch", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{"name": "changed"}}); err != nil {
		t.Fatalf("PATCH write: %v", err)
	}
	if strings.Join(methods, ",") != "POST,PATCH" {
		t.Fatalf("methods: %v", methods)
	}
}

func TestClientResponseRepresentations(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(status int, contentType, body string, limit int64) (*Response, error) {
		h := make(http.Header)
		h.Set("Content-Type", contentType)
		h.Set("X-Request-Id", "id")
		h.Set("X-Rate-Limit", "user-hour-lim:3500;user-hour-rem:500;")
		h.Set("Set-Cookie", "secret")
		h.Set("Authorization", "secret")
		client := &Client{Catalog: c, Tokens: token("token"), MaxResponseBytes: limit, HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		return client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}})
	}
	r, err := invoke(200, "application/json", `{"ok":true}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Body.(map[string]any); !ok {
		t.Fatalf("JSON body was not structured: %#v", r.Body)
	}
	if r.Headers["X-Request-Id"] != "id" || r.Headers["X-Rate-Limit"] != "user-hour-lim:3500;user-hour-rem:500;" || r.Headers["Set-Cookie"] != "" || r.Headers["Authorization"] != "" {
		t.Fatalf("unsafe headers leaked: %#v", r.Headers)
	}
	r, err = invoke(200, "application/vnd.api+json", `{"data":{}}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Body.(map[string]any); !ok {
		t.Fatal("vendor JSON was not structured")
	}
	r, err = invoke(200, "text/plain", "hello", 0)
	if err != nil || r.Body != "hello" {
		t.Fatalf("text: %#v %v", r, err)
	}
	r, err = invoke(204, "", "", 0)
	if err != nil || r.Body != nil {
		t.Fatalf("empty: %#v %v", r, err)
	}
	if _, err = invoke(200, "application/octet-stream", "abc", 0); err == nil {
		t.Fatal("binary body accepted")
	}
	if _, err = invoke(200, "text/plain", "long", 2); err == nil {
		t.Fatal("oversize body accepted")
	}
	if _, err = invoke(400, "application/json", `{"error":"bad"}`, 0); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatal("non-2xx accepted")
	}
}
