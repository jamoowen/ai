package googleplay

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testToken string

func (t testToken) Token(context.Context) (string, error) { return string(t), nil }

type testRoundTrip func(*http.Request) (*http.Response, error)

func (f testRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCheckedInDiscoveryDocumentIsCompatible(t *testing.T) {
	catalog, err := LoadCatalogFile("../api/google/androidpublisher.v3.discovery.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(catalog.operations); got != 145 {
		t.Fatalf("operations = %d, want 145", got)
	}
	if _, err := catalog.Operation("androidpublisher.reviews.list"); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.DescribeSchema("Review"); err != nil {
		t.Fatal(err)
	}
}

func TestClientExpandsReservedPathAndRejectsUnsafeInput(t *testing.T) {
	const doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","schemas":{"Request":{"type":"object","properties":{"name":{"type":"string"}}}},"resources":{"users":{"methods":{"list":{"id":"users.list","path":"v3/{+parent}/users","httpMethod":"GET","parameters":{"parent":{"location":"path","required":true,"type":"string","pattern":"^developers/[^/]+$"}},"response":{"$ref":"Request"}},"update":{"id":"users.update","path":"v3/{+parent}/users/{email}","httpMethod":"PUT","parameters":{"parent":{"location":"path","required":true,"type":"string","pattern":"^developers/[^/]+$"},"email":{"location":"path","required":true,"type":"string"}},"request":{"$ref":"Request"}}}}}}`
	catalog, err := LoadCatalog([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{Catalog: catalog, Tokens: testToken("token"), HTTPClient: &http.Client{Transport: testRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "androidpublisher.googleapis.com" || r.URL.EscapedPath() != "/v3/developers/123/users" {
			t.Fatalf("unsafe URL: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("missing token")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "users.list", PathParameters: map[string]string{"parent": "developers/123"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "users.list", PathParameters: map[string]string{"parent": "developers/123?x=1"}}); err == nil {
		t.Fatal("query injection accepted")
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "users.update", PathParameters: map[string]string{"parent": "developers/123", "email": "a"}, Body: map[string]any{"unknown": true}}); err == nil {
		t.Fatal("unknown body field accepted")
	}
	if calls != 1 {
		t.Fatalf("unsafe input reached transport: %d", calls)
	}
}

func TestLoadCatalogRejectsDuplicateUnsafeAndUnsupportedMethods(t *testing.T) {
	for _, doc := range []string{
		`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"same","path":"v3/a","httpMethod":"GET"},"b":{"id":"same","path":"v3/b","httpMethod":"GET"}}}`,
		`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"bad","path":"https://example.com/x","httpMethod":"GET"}}}`,
		`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"bad","path":"v3/a","httpMethod":"HEAD"}}}`,
		`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"bad","path":"v3/{missing","httpMethod":"GET"}}}`,
		`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"bad","path":"v3/a","httpMethod":"GET","parameters":{"unused":{"location":"path","type":"string"}}}}}`,
	} {
		if _, err := LoadCatalog([]byte(doc)); err == nil {
			t.Fatalf("invalid document accepted: %s", doc)
		}
	}
}

func TestClientClassifiesAllSupportedMethodsAndRejectsMediaBeforeTransport(t *testing.T) {
	const doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","schemas":{"Request":{"type":"object","properties":{}}},"methods":{"get":{"id":"get","path":"v3/get","httpMethod":"GET"},"post":{"id":"post","path":"v3/post","httpMethod":"POST","request":{"$ref":"Request"}},"put":{"id":"put","path":"v3/put","httpMethod":"PUT","request":{"$ref":"Request"}},"patch":{"id":"patch","path":"v3/patch","httpMethod":"PATCH","request":{"$ref":"Request"}},"delete":{"id":"delete","path":"v3/delete","httpMethod":"DELETE"},"media":{"id":"media","path":"v3/media","httpMethod":"POST","supportsMediaUpload":true,"request":{"$ref":"Request"}}}}`
	catalog, err := LoadCatalog([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &Client{Catalog: catalog, Tokens: testToken("token"), HTTPClient: &http.Client{Transport: testRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}}
	for _, test := range []struct {
		class OperationClass
		id    string
		body  map[string]any
	}{
		{ReadOperation, "get", nil}, {WriteOperation, "post", map[string]any{}}, {WriteOperation, "put", map[string]any{}}, {WriteOperation, "patch", map[string]any{}}, {DeleteOperation, "delete", nil},
	} {
		if _, err := client.Invoke(context.Background(), test.class, Invocation{OperationID: test.id, Body: test.body}); err != nil {
			t.Fatalf("%s: %v", test.id, err)
		}
	}
	if _, err := client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "media", Body: map[string]any{}}); err == nil {
		t.Fatal("media upload accepted")
	}
	if calls != 5 {
		t.Fatalf("media operation reached transport: %d calls", calls)
	}
}

func TestClientRejectsRedirectOffFixedHost(t *testing.T) {
	const doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"get":{"id":"get","path":"v3/get","httpMethod":"GET"}}}`
	catalog, err := LoadCatalog([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{Catalog: catalog, Tokens: testToken("token"), HTTPClient: &http.Client{Transport: testRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"https://example.com/redirect"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, err := client.Invoke(context.Background(), ReadOperation, Invocation{OperationID: "get"}); err == nil {
		t.Fatal("cross-host redirect accepted")
	}
}
