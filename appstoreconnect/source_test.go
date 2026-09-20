package appstoreconnect

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCatalogSourceRejectsHTTPSDowngradeRedirect(t *testing.T) {
	old := sourceHTTPClient
	defer func() { sourceHTTPClient = old }()
	calls := 0
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		h := make(http.Header)
		h.Set("Location", "http://example.com/spec.json")
		return &http.Response{StatusCode: http.StatusFound, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if _, err := LoadCatalogSource("https://example.com/spec.json"); err == nil {
		t.Fatal("HTTPS downgrade accepted")
	}
	if calls != 1 {
		t.Fatalf("redirect issued %d requests", calls)
	}
}
