package appstoreconnect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultSpecPath = "api/apple/app-store-connect.openapi.json"
	defaultSpecURL  = "https://api.github.com/repos/jamoowen/ai/contents/api/apple/app-store-connect.openapi.json?ref=main"
	maxSpecBytes    = 16 << 20
)

var (
	sourceHTTPClient = &http.Client{Timeout: 20 * time.Second}
	userCacheDir     = os.UserCacheDir
	writeCacheFile   = atomicWriteFile
)

type cachedSpec struct {
	ETag string          `json:"etag"`
	Spec json.RawMessage `json:"spec"`
}

// LoadCatalogSource loads an explicitly configured local path or HTTPS URL. When
// source is empty, it uses the cached GitHub specification and conditionally
// refreshes it.
func LoadCatalogSource(source string) (*Catalog, error) {
	catalog, _, err := LoadCatalogSourceWithDiagnostics(source)
	return catalog, err
}

// LoadCatalogSourceWithDiagnostics is LoadCatalogSource with startup diagnostics
// suitable for writing to stderr by a command-line caller.
func LoadCatalogSourceWithDiagnostics(source string) (*Catalog, []string, error) {
	if source == "" {
		return loadDefaultCatalog()
	}
	return loadConfiguredCatalog(source)
}

func loadConfiguredCatalog(source string) (*Catalog, []string, error) {
	u, err := url.Parse(source)
	if err == nil && u.Scheme != "" {
		if u.Scheme != "https" {
			return nil, nil, fmt.Errorf("OpenAPI URL must use HTTPS")
		}
		b, _, _, err := downloadSpec(source, "")
		if err != nil {
			return nil, nil, err
		}
		catalog, err := LoadCatalog(b)
		return catalog, nil, err
	}
	if strings.HasPrefix(source, "//") {
		return nil, nil, fmt.Errorf("OpenAPI URL must use HTTPS")
	}
	catalog, err := LoadCatalogFile(source)
	return catalog, nil, err
}

func loadDefaultCatalog() (*Catalog, []string, error) {
	dir, err := cacheDirectory()
	if err != nil {
		return loadUncachedDefaultCatalog(fmt.Errorf("locate OpenAPI cache: %w", err))
	}

	cached, record, cacheErr := loadCachedCatalog(dir)
	if cacheErr != nil {
		record = cachedSpec{}
	}

	b, status, etag, err := downloadSpec(defaultSpecURL, record.ETag)
	if err != nil {
		return fallbackToCache(cached, cacheErr, fmt.Errorf("refresh OpenAPI specification: %w", err))
	}
	if status == http.StatusNotModified {
		if cached == nil {
			return nil, nil, errors.New("OpenAPI specification returned 304 but no valid cached specification is available")
		}
		return cached, nil, nil
	}

	fresh, err := LoadCatalog(b)
	if err != nil {
		return fallbackToCache(cached, cacheErr, fmt.Errorf("validate downloaded OpenAPI specification: %w", err))
	}
	if err := storeCachedSpec(dir, b, etag); err != nil {
		return fresh, []string{fmt.Sprintf("could not cache OpenAPI specification: %v; using the newly downloaded specification", err)}, nil
	}
	return fresh, nil, nil
}

func loadUncachedDefaultCatalog(cacheErr error) (*Catalog, []string, error) {
	b, status, _, err := downloadSpec(defaultSpecURL, "")
	if err != nil {
		return nil, nil, fmt.Errorf("refresh OpenAPI specification: %w; cache unavailable: %v", err, cacheErr)
	}
	if status == http.StatusNotModified {
		return nil, nil, errors.New("OpenAPI specification returned 304 but no valid cached specification is available")
	}
	catalog, err := LoadCatalog(b)
	if err != nil {
		return nil, nil, fmt.Errorf("validate downloaded OpenAPI specification: %w", err)
	}
	return catalog, []string{fmt.Sprintf("%v; using the newly downloaded OpenAPI specification without a cache", cacheErr)}, nil
}

func fallbackToCache(cached *Catalog, cacheErr, refreshErr error) (*Catalog, []string, error) {
	if cached != nil {
		return cached, []string{fmt.Sprintf("%v; using cached OpenAPI specification", refreshErr)}, nil
	}
	if cacheErr != nil {
		return nil, nil, fmt.Errorf("%v; no valid cached OpenAPI specification: %w", refreshErr, cacheErr)
	}
	return nil, nil, fmt.Errorf("%v; no cached OpenAPI specification is available", refreshErr)
}

// downloadSpec returns an empty body for a 304 response.
func downloadSpec(source, etag string) ([]byte, int, string, error) {
	client := clientWithRedirectPolicy(sourceHTTPClient, func(req *http.Request) bool {
		return req.URL.Scheme == "https"
	}, "OpenAPI redirects must use HTTPS")
	req, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, 0, "", err
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if source == defaultSpecURL {
		req.Header.Set("Accept", "application/vnd.github.raw+json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, resp.StatusCode, resp.Header.Get("ETag"), nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, "", fmt.Errorf("download OpenAPI spec: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes+1))
	if err != nil {
		return nil, 0, "", err
	}
	if len(b) > maxSpecBytes {
		return nil, 0, "", fmt.Errorf("OpenAPI spec exceeds 16 MiB")
	}
	return b, resp.StatusCode, resp.Header.Get("ETag"), nil
}

func cacheDirectory() (string, error) {
	base, err := userCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "appstoreconnect-mcp"), nil
}

func loadCachedCatalog(dir string) (*Catalog, cachedSpec, error) {
	b, err := os.ReadFile(filepath.Join(dir, "openapi-cache.json"))
	if err != nil {
		return nil, cachedSpec{}, err
	}
	var record cachedSpec
	if err := json.Unmarshal(b, &record); err != nil {
		return nil, cachedSpec{}, err
	}
	if len(record.Spec) == 0 {
		return nil, cachedSpec{}, errors.New("OpenAPI cache has no specification")
	}
	catalog, err := LoadCatalog(record.Spec)
	if err != nil {
		return nil, cachedSpec{}, err
	}
	return catalog, record, nil
}

func storeCachedSpec(dir string, spec []byte, etag string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	record, err := json.Marshal(cachedSpec{ETag: etag, Spec: spec})
	if err != nil {
		return err
	}
	return writeCacheFile(filepath.Join(dir, "openapi-cache.json"), record)
}

func atomicWriteFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".appstoreconnect-mcp-*")
	if err != nil {
		return err
	}
	temporaryPath := f.Name()
	defer os.Remove(temporaryPath)
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
