package googleplay

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachedDiscoveryRoundTripAndSourceRestrictions(t *testing.T) {
	dir := t.TempDir()
	doc := []byte(`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"a.get","path":"v3/a","httpMethod":"GET"}}}`)
	if err := storeCachedCatalog(dir, doc, `"etag"`); err != nil {
		t.Fatal(err)
	}
	catalog, record, err := loadCachedCatalog(dir)
	if err != nil || record.ETag != `"etag"` || len(catalog.operations) != 1 {
		t.Fatalf("cache: catalog=%v record=%#v err=%v", catalog, record, err)
	}
	path := filepath.Join(dir, "document.json")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadConfiguredCatalog("http://example.com/document.json"); err == nil {
		t.Fatal("HTTP source accepted")
	}
	if _, _, err := loadConfiguredCatalog(path); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultSourceUsesETagCacheAndFallsBackAfterRefreshFailure(t *testing.T) {
	doc := []byte(`{"kind":"discovery#restDescription","name":"androidpublisher","version":"v3","methods":{"a":{"id":"a.get","path":"v3/a","httpMethod":"GET"}}}`)
	oldURL, oldClient, oldCacheDir := defaultDiscoveryURL, sourceHTTPClient, userCacheDir
	defer func() { defaultDiscoveryURL, sourceHTTPClient, userCacheDir = oldURL, oldClient, oldCacheDir }()
	requests := 0
	phase := 0
	var conditionalETag string
	defaultDiscoveryURL = "https://discovery.example/document"
	sourceHTTPClient = &http.Client{Transport: testRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		if phase == 0 {
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Etag": []string{`"one"`}}, Body: io.NopCloser(strings.NewReader(string(doc)))}, nil
		}
		if phase == 1 {
			conditionalETag = r.Header.Get("If-None-Match")
			return &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	cacheDir := t.TempDir()
	userCacheDir = func() (string, error) { return cacheDir, nil }
	if _, _, err := loadDefaultCatalog(); err != nil {
		t.Fatal(err)
	}
	phase = 1
	if _, diagnostics, err := loadDefaultCatalog(); err != nil || len(diagnostics) != 0 || requests != 2 || conditionalETag != `"one"` {
		t.Fatalf("304 cache reuse: diagnostics=%v requests=%d etag=%q err=%v", diagnostics, requests, conditionalETag, err)
	}
	phase = 2
	if catalog, diagnostics, err := loadDefaultCatalog(); err != nil || catalog == nil || len(diagnostics) != 1 {
		t.Fatalf("cache fallback: catalog=%v diagnostics=%v err=%v", catalog, diagnostics, err)
	}
}

func TestDiscoveryRejectsHTTPSDowngradeRedirect(t *testing.T) {
	oldClient := sourceHTTPClient
	defer func() { sourceHTTPClient = oldClient }()
	sourceHTTPClient = &http.Client{Transport: testRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{"http://example.com/document"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}
	if _, _, _, err := downloadDiscovery("https://example.com/document", ""); err == nil {
		t.Fatal("HTTPS downgrade redirect accepted")
	}
}
