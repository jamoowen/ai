package appstoreconnect

import (
	"encoding/json"
	"errors"
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
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
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

func TestDefaultCatalogCachesAndUsesConditionalRequest(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	requests := 0
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.URL.String() != defaultSpecURL {
			t.Fatalf("requested %s", req.URL)
		}
		if got := req.Header.Get("Accept"); got != "application/vnd.github.raw+json" {
			t.Fatalf("Accept = %q", got)
		}
		if requests == 1 {
			if got := req.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("first request If-None-Match = %q", got)
			}
			resp := response(http.StatusOK, fixture)
			resp.Header.Set("ETag", `"v1"`)
			return resp, nil
		}
		if got := req.Header.Get("If-None-Match"); got != `"v1"` {
			t.Fatalf("If-None-Match = %q", got)
		}
		return response(http.StatusNotModified, ""), nil
	})}
	if _, err := LoadCatalogSource(""); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalogSource(""); err != nil {
		t.Fatal(err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d", requests)
	}
	dir := filepath.Join(cache, "appstoreconnect-mcp")
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("cache directory permissions = %#o", dirInfo.Mode().Perm())
	}
	for _, name := range []string{"openapi-cache.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions = %#o", name, info.Mode().Perm())
		}
	}
}

func TestDefaultCatalogReplacesChangedSpec(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	changed := strings.Replace(fixture, `"version":"1"`, `"version":"2"`, 1)
	requests := 0
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		resp := response(http.StatusOK, fixture)
		if requests == 2 {
			if got := req.Header.Get("If-None-Match"); got != `"v1"` {
				t.Fatalf("If-None-Match = %q", got)
			}
			resp = response(http.StatusOK, changed)
			resp.Header.Set("ETag", `"v2"`)
		} else {
			resp.Header.Set("ETag", `"v1"`)
		}
		return resp, nil
	})}
	if _, err := LoadCatalogSource(""); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCatalogSource(""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(cache, "appstoreconnect-mcp", "openapi-cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record cachedSpec
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if string(record.Spec) != changed {
		t.Fatal("cache was not replaced")
	}
}

func TestDefaultCatalogFallsBackToValidCache(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	dir := filepath.Join(cache, "appstoreconnect-mcp")
	if err := storeCachedSpec(dir, []byte(fixture), `"v1"`); err != nil {
		t.Fatal(err)
	}
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, "{"), nil
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
}

func TestDefaultCatalogFailsWithoutValidCache(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(http.StatusServiceUnavailable, ""), nil
	})}
	if _, err := LoadCatalogSource(""); err == nil {
		t.Fatal("first launch succeeded without a cache")
	}
}

func TestDefaultCatalogUsesFreshSpecWhenCachingFails(t *testing.T) {
	cacheFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(cacheFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cacheFile, nil }
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, fixture), nil
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
}

func TestDefaultCatalogKeepsPreviousCacheWhenReplacementFails(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir, oldWriteCacheFile := sourceHTTPClient, userCacheDir, writeCacheFile
	t.Cleanup(func() { sourceHTTPClient, userCacheDir, writeCacheFile = oldClient, oldCacheDir, oldWriteCacheFile })
	userCacheDir = func() (string, error) { return cache, nil }
	dir := filepath.Join(cache, "appstoreconnect-mcp")
	if err := storeCachedSpec(dir, []byte(fixture), `"v1"`); err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(fixture, `"version":"1"`, `"version":"2"`, 1)
	writeCacheFile = func(string, []byte) error { return errors.New("disk full") }
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		resp := response(http.StatusOK, changed)
		resp.Header.Set("ETag", `"v2"`)
		return resp, nil
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
	writeCacheFile = oldWriteCacheFile
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "openapi-cache.json"))
	if err != nil {
		t.Fatal(err)
	}
	var record cachedSpec
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if string(record.Spec) != fixture || record.ETag != `"v1"` {
		t.Fatal("failed replacement changed the previous cache")
	}
}

func TestDefaultCatalogIgnoresMalformedCache(t *testing.T) {
	cache := t.TempDir()
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	dir := filepath.Join(cache, "appstoreconnect-mcp")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openapi-cache.json"), []byte(`{"etag":"v1","spec":`), 0o600); err != nil {
		t.Fatal(err)
	}
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Fatalf("malformed cache supplied an ETag: %q", got)
		}
		return response(http.StatusNotModified, ""), nil
	})}
	if _, err := LoadCatalogSource(""); err == nil {
		t.Fatal("304 with malformed cache succeeded")
	}
}

func TestDefaultCatalogUsesFreshSpecWhenCacheDirectoryCannotBeLocated(t *testing.T) {
	oldClient, oldCacheDir := sourceHTTPClient, userCacheDir
	t.Cleanup(func() { sourceHTTPClient, userCacheDir = oldClient, oldCacheDir })
	userCacheDir = func() (string, error) { return "", errors.New("cache unavailable") }
	sourceHTTPClient = &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("If-None-Match"); got != "" {
			t.Fatalf("uncached fetch supplied ETag: %q", got)
		}
		return response(http.StatusOK, fixture), nil
	})}
	if _, diagnostics, err := LoadCatalogSourceWithDiagnostics(""); err != nil || len(diagnostics) != 1 {
		t.Fatalf("catalog diagnostics=%q err=%v", diagnostics, err)
	}
}

func TestCatalogSourceExplicitOverrideDoesNotUseCache(t *testing.T) {
	cache := t.TempDir()
	path := filepath.Join(t.TempDir(), "spec.json")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	oldCacheDir := userCacheDir
	t.Cleanup(func() { userCacheDir = oldCacheDir })
	userCacheDir = func() (string, error) { return cache, nil }
	if _, err := LoadCatalogSource(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cache, "appstoreconnect-mcp")); !os.IsNotExist(err) {
		t.Fatalf("explicit source created cache: %v", err)
	}
}
