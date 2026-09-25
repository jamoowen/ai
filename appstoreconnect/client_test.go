package appstoreconnect

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestClientHTTPClientUsesDefaultTimeoutWithoutMutatingCustomClient(t *testing.T) {
	if got := (&Client{}).httpClient().Timeout; got != defaultRequestTimeout {
		t.Fatalf("default timeout = %s, want %s", got, defaultRequestTimeout)
	}
	base := &http.Client{}
	if got := (&Client{HTTPClient: base}).httpClient().Timeout; got != defaultRequestTimeout {
		t.Fatalf("zero custom timeout = %s, want %s", got, defaultRequestTimeout)
	}
	if base.Timeout != 0 {
		t.Fatalf("custom client was mutated: %s", base.Timeout)
	}
	base.Timeout = 3 * time.Second
	if got := (&Client{HTTPClient: base}).httpClient().Timeout; got != base.Timeout {
		t.Fatalf("custom timeout = %s, want %s", got, base.Timeout)
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
	r, err = invoke(429, "text/plain", "long", 2)
	if err == nil || r == nil || r.Status != 429 || r.ContentType != "text/plain" || r.Headers["X-Request-Id"] != "id" || r.Body != nil {
		t.Fatalf("oversize body lost response metadata: response=%#v err=%v", r, err)
	}
	if _, err = invoke(400, "application/json", `{"error":"bad"}`, 0); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatal("non-2xx accepted")
	}
	r, err = invoke(429, "", "unavailable", 0)
	if err == nil || !strings.Contains(err.Error(), "App Store Connect returned HTTP 429") || r.Status != 429 {
		t.Fatalf("non-2xx parse failure lost status: response=%#v err=%v", r, err)
	}
	large := `{"error":"` + strings.Repeat("x", 8<<10) + `"}`
	r, err = invoke(500, "application/json", large, 16<<10)
	if err == nil || !strings.Contains(err.Error(), "(truncated)") || len(err.Error()) > 5<<10 {
		t.Fatalf("non-2xx detail was not bounded: len=%d err=%v", len(err.Error()), err)
	}
	if body, ok := r.Body.(map[string]any); !ok || len(body["error"].(string)) != 8<<10 {
		t.Fatalf("response body was truncated: %#v", r.Body)
	}
}

func TestClientQueryNullAndUnsupportedValuesDoNotReachTransport(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Query().Has("mode") {
			t.Fatalf("null query parameter was sent: %s", r.URL)
		}
		return response(200, "{}"), nil
	})}}
	in := Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}, Query: map[string]any{"mode": nil}}
	if _, err := client.Invoke(context.Background(), ReadOperation, in); err != nil {
		t.Fatalf("optional null: %v", err)
	}
	for _, query := range []map[string]any{{"mode": map[string]any{"bad": "value"}}, {"include": []any{"ok", nil}}, {"include": []any{map[string]any{"bad": "value"}}}} {
		in.Query = query
		if _, err := client.Invoke(context.Background(), ReadOperation, in); err == nil || !strings.Contains(err.Error(), "query parameter") {
			t.Fatalf("unsupported query accepted: %#v, err=%v", query, err)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid queries reached transport: %d calls", calls)
	}
}

func TestClientCanInvokeConcurrentlyWithCatalogRouter(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, "{}"), nil
	})}}
	var group sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "x"}})
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}
