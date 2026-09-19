package appstoreconnect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const fixture = `{"openapi":"3.0.1","info":{"title":"test","version":"1"},"paths":{"/v1/apps/{id}":{"parameters":[{"name":"id","in":"path","required":true,"schema":{"type":"string"}}],"get":{"operationId":"apps_get","tags":["Apps"],"parameters":[{"name":"filter[name]","in":"query","style":"form","explode":false,"schema":{"type":"array","items":{"type":"string"}}}],"responses":{"200":{"description":"ok"}}},"patch":{"operationId":"apps_patch","requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}}}},"responses":{"200":{"description":"ok"}}}}}}`

func TestCatalogAndClient(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	got := c.Search("apps get", "GET", 10)
	if len(got) != 1 || got[0].OperationID != "apps_get" {
		t.Fatalf("search: %#v", got)
	}
	if got := c.Search("get filter name", "", 10); len(got) != 1 {
		t.Fatalf("method/parameter search: %#v", got)
	}
	d, err := c.Describe("apps_get")
	if err != nil || d["operation"] == nil {
		t.Fatalf("describe: %v %#v", err, d)
	}
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.appstoreconnect.apple.com" || r.URL.Query().Get("filter[name]") != "a,b" {
			t.Fatalf("bad request %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatal("missing auth")
		}
		return response(200, "{}"), nil
	})
	client := &Client{Catalog: c, Tokens: token("token"), HTTPClient: &http.Client{Transport: rt}}
	if _, err := client.Invoke(context.Background(), "read", Invocation{OperationID: "apps_get", PathParameters: map[string]string{"id": "a/b"}, Query: map[string]any{"filter[name]": []string{"a", "b"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Invoke(context.Background(), "read", Invocation{OperationID: "apps_patch"}); err == nil {
		t.Fatal("write accepted as read")
	}
	if _, err := client.Invoke(context.Background(), "write", Invocation{OperationID: "apps_patch", PathParameters: map[string]string{"id": "x"}, Body: map[string]any{}}); err == nil {
		t.Fatal("invalid body reached transport")
	}
}
func TestJWTClaims(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "key.p8")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: b}), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := NewES256TokenSource("kid", "issuer", p, 5*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	raw, err := s.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil || !parsed.Valid {
		t.Fatal(err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if parsed.Header["alg"] != "ES256" || parsed.Header["kid"] != "kid" || claims["iss"] != "issuer" || claims["aud"] != "appstoreconnect-v1" || claims["sub"] != nil || claims["exp"].(float64)-claims["iat"].(float64) != 300 {
		t.Fatalf("claims %#v", claims)
	}
	individual, err := NewES256TokenSource("kid", "", p, 5*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	raw, err = individual.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = jwt.Parse(raw, func(token *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	claims = parsed.Claims.(jwt.MapClaims)
	if claims["sub"] != "user" || claims["iss"] != nil {
		t.Fatalf("individual claims %#v", claims)
	}
	if _, err := NewES256TokenSource("kid", "", p, 21*time.Minute, nil); err == nil {
		t.Fatal("overlong lifetime accepted")
	}
	if _, err := NewES256TokenSource("kid", "", p, -time.Minute, nil); err == nil {
		t.Fatal("negative lifetime accepted")
	}
}

func TestCheckedInAppleSpecCompatibility(t *testing.T) {
	c, err := LoadCatalogFile("../api/apple/app-store-connect.openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Operation("apps_getCollection"); err != nil {
		t.Fatal(err)
	}
	if len(c.Search("create app", "POST", 10)) == 0 {
		t.Fatal("expected a POST operation")
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

func TestClientResponseRepresentations(t *testing.T) {
	c, err := LoadCatalog([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(status int, contentType, body string, limit int64) (*Response, error) {
		h := make(http.Header)
		h.Set("Content-Type", contentType)
		h.Set("X-Request-Id", "id")
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
	if _, err = invoke(400, "application/json", `{"error":"bad"}`, 0); err == nil {
		t.Fatal("non-2xx accepted")
	}
}

func TestCatalogSourceRejectsHTTP(t *testing.T) {
	if _, err := LoadCatalogSource("http://example.com/spec.json"); err == nil {
		t.Fatal("HTTP source accepted")
	}
}

func TestCatalogSourceLoadsLocalAndCapsHTTPS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalogSource(path); err != nil {
		t.Fatal(err)
	}
	old := sourceHTTPClient
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { return response(200, strings.Repeat("x", 16<<20+1)), nil })}
	t.Cleanup(func() { sourceHTTPClient = old })
	if _, err := LoadCatalogSource("https://example.com/spec.json"); err == nil {
		t.Fatal("oversized source accepted")
	}
}

func TestCatalogRejectsMalformedMissingAndDuplicateOperationIDs(t *testing.T) {
	if _, err := LoadCatalog([]byte("{")); err == nil {
		t.Fatal("malformed spec accepted")
	}
	missing := strings.Replace(fixture, `"operationId":"apps_get",`, "", 1)
	if _, err := LoadCatalog([]byte(missing)); err == nil {
		t.Fatal("missing operation ID accepted")
	}
	duplicate := strings.Replace(fixture, "apps_patch", "apps_get", 1)
	if _, err := LoadCatalog([]byte(duplicate)); err == nil {
		t.Fatal("duplicate operation ID accepted")
	}
}

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(q *http.Request) (*http.Response, error) { return r(q) }
func response(code int, body string) *http.Response {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}
