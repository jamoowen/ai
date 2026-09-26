package googleplay

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testToken string

func (t testToken) Token(context.Context) (string, error) { return string(t), nil }

type countingToken struct{ calls int }

func (t *countingToken) Token(context.Context) (string, error) {
	t.calls++
	return "token", nil
}

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

func TestLoadCatalogRejectsMalformedOptionalSections(t *testing.T) {
	for _, section := range []string{"schemas", "methods", "resources"} {
		doc := `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"get":{"id":"get","path":"v3/get","httpMethod":"GET"}}}`
		if section == "schemas" {
			doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","schemas":[],"methods":{"get":{"id":"get","path":"v3/get","httpMethod":"GET"}}}`
		} else if section == "methods" {
			doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":[]}`
		} else {
			doc = `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"get":{"id":"get","path":"v3/get","httpMethod":"GET"}},"resources":[]}`
		}
		if _, err := LoadCatalog([]byte(doc)); err == nil || !strings.Contains(err.Error(), "Discovery "+section+" must be an object") {
			t.Fatalf("%s: err = %v", section, err)
		}
	}
}

func TestLoadCatalogRejectsNonObjectRequest(t *testing.T) {
	doc := `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"update":{"id":"update","path":"v3/update","httpMethod":"PUT","request":null}}}`
	if _, err := LoadCatalog([]byte(doc)); err == nil || !strings.Contains(err.Error(), "Discovery method request must be an object") {
		t.Fatalf("err = %v", err)
	}
}

func TestClientRejectsUnusableRequestSchemaBeforeTokenAndTransport(t *testing.T) {
	for name, test := range map[string]struct {
		doc  string
		want string
	}{
		"missing reference": {
			doc:  `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"update":{"id":"update","path":"v3/update","httpMethod":"PUT","request":{}}}}`,
			want: "request schema reference is required",
		},
		"unresolved reference": {
			doc:  `{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"update":{"id":"update","path":"v3/update","httpMethod":"PUT","request":{"$ref":"Missing"}}}}`,
			want: `request schema "Missing" is missing`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			catalog, err := LoadCatalog([]byte(test.doc))
			if err != nil {
				t.Fatal(err)
			}
			tokens := &countingToken{}
			calls := 0
			client := &Client{Catalog: catalog, Tokens: tokens, HTTPClient: &http.Client{Transport: testRoundTrip(func(*http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("transport must not be called")
			})}}
			_, err = client.Invoke(context.Background(), WriteOperation, Invocation{OperationID: "update", Body: map[string]any{}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err = %v", err)
			}
			if tokens.calls != 0 || calls != 0 {
				t.Fatalf("invalid request schema reached token=%d transport=%d", tokens.calls, calls)
			}
		})
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
