package appstoreconnect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if claims["iss"] != "issuer" || claims["aud"] != "appstoreconnect-v1" {
		t.Fatalf("claims %#v", claims)
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

type token string

func (t token) Token(context.Context) (string, error) { return string(t), nil }

type roundTrip func(*http.Request) (*http.Response, error)

func (r roundTrip) RoundTrip(q *http.Request) (*http.Response, error) { return r(q) }
func response(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: ioNop(strings.NewReader(body))}
}

type nop struct{ *strings.Reader }

func (n nop) Close() error         { return nil }
func ioNop(r *strings.Reader) *nop { return &nop{r} }
