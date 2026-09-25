package googleplay

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
	DefaultDiscoveryPath = "api/google/androidpublisher.v3.discovery.json"
	maxSpecBytes         = 16 << 20
)

var (
	defaultDiscoveryURL = "https://androidpublisher.googleapis.com/$discovery/rest?version=v3"
	sourceHTTPClient    = &http.Client{Timeout: 20 * time.Second}
	userCacheDir        = os.UserCacheDir
	writeCacheFile      = atomicWriteFile
)

type cachedDiscovery struct {
	ETag     string          `json:"etag"`
	Document json.RawMessage `json:"document"`
}

func LoadCatalogSource(source string) (*Catalog, error) {
	c, _, e := LoadCatalogSourceWithDiagnostics(source)
	return c, e
}

func LoadCatalogSourceWithDiagnostics(source string) (*Catalog, []string, error) {
	if source != "" {
		return loadConfiguredCatalog(source)
	}
	return loadDefaultCatalog()
}

func loadConfiguredCatalog(source string) (*Catalog, []string, error) {
	u, e := url.Parse(source)
	if e == nil && u.Scheme != "" {
		if u.Scheme != "https" || u.Host == "" {
			return nil, nil, fmt.Errorf("Discovery URL must use HTTPS")
		}
		b, _, _, e := downloadDiscovery(source, "")
		if e != nil {
			return nil, nil, e
		}
		c, e := LoadCatalog(b)
		return c, nil, e
	}
	if strings.HasPrefix(source, "//") {
		return nil, nil, fmt.Errorf("Discovery URL must use HTTPS")
	}
	c, e := LoadCatalogFile(source)
	return c, nil, e
}

func loadDefaultCatalog() (*Catalog, []string, error) {
	dir, e := cacheDirectory()
	if e != nil {
		return loadUncachedDefault(e)
	}
	cached, record, cacheErr := loadCachedCatalog(dir)
	if cacheErr != nil {
		record = cachedDiscovery{}
	}
	b, status, etag, e := downloadDiscovery(defaultDiscoveryURL, record.ETag)
	if e != nil {
		return fallback(cached, cacheErr, fmt.Errorf("refresh Discovery document: %w", e))
	}
	if status == http.StatusNotModified {
		if cached == nil {
			return nil, nil, errors.New("Discovery document returned 304 but no valid cached document is available")
		}
		return cached, nil, nil
	}
	fresh, e := LoadCatalog(b)
	if e != nil {
		return fallback(cached, cacheErr, fmt.Errorf("validate downloaded Discovery document: %w", e))
	}
	if e := storeCachedCatalog(dir, b, etag); e != nil {
		return fresh, []string{fmt.Sprintf("could not cache Discovery document: %v; using the newly downloaded document", e)}, nil
	}
	return fresh, nil, nil
}

func loadUncachedDefault(e error) (*Catalog, []string, error) {
	b, status, _, err := downloadDiscovery(defaultDiscoveryURL, "")
	if err != nil {
		return nil, nil, fmt.Errorf("refresh Discovery document: %w; cache unavailable: %v", err, e)
	}
	if status == 304 {
		return nil, nil, errors.New("Discovery document returned 304 but cache is unavailable")
	}
	c, err := LoadCatalog(b)
	if err != nil {
		return nil, nil, err
	}
	return c, []string{fmt.Sprintf("%v; using downloaded Discovery document without a cache", e)}, nil
}

func fallback(c *Catalog, cacheErr, refreshErr error) (*Catalog, []string, error) {
	if c != nil {
		return c, []string{fmt.Sprintf("%v; using cached Discovery document", refreshErr)}, nil
	}
	if cacheErr != nil {
		return nil, nil, fmt.Errorf("%v; no valid cached Discovery document: %w", refreshErr, cacheErr)
	}
	return nil, nil, fmt.Errorf("%v; no cached Discovery document is available", refreshErr)
}

func downloadDiscovery(source, etag string) ([]byte, int, string, error) {
	client := clientWithRedirectPolicy(sourceHTTPClient, func(r *http.Request) bool { return r.URL.Scheme == "https" }, "Discovery redirects must use HTTPS")
	req, e := http.NewRequest(http.MethodGet, source, nil)
	if e != nil {
		return nil, 0, "", e
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, e := client.Do(req)
	if e != nil {
		return nil, 0, "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode == 304 {
		return nil, 304, resp.Header.Get("ETag"), nil
	}
	if resp.StatusCode != 200 {
		return nil, resp.StatusCode, "", fmt.Errorf("download Discovery document: HTTP %d", resp.StatusCode)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, maxSpecBytes+1))
	if e != nil {
		return nil, 0, "", e
	}
	if len(b) > maxSpecBytes {
		return nil, 0, "", fmt.Errorf("Discovery document exceeds 16 MiB")
	}
	return b, resp.StatusCode, resp.Header.Get("ETag"), nil
}

func cacheDirectory() (string, error) {
	b, e := userCacheDir()
	return filepath.Join(b, "googleplay-mcp"), e
}

func loadCachedCatalog(dir string) (*Catalog, cachedDiscovery, error) {
	b, e := os.ReadFile(filepath.Join(dir, "discovery-cache.json"))
	if e != nil {
		return nil, cachedDiscovery{}, e
	}
	var r cachedDiscovery
	if e = json.Unmarshal(b, &r); e != nil {
		return nil, r, e
	}
	c, e := LoadCatalog(r.Document)
	return c, r, e
}

func storeCachedCatalog(dir string, b []byte, etag string) error {
	if e := os.MkdirAll(dir, 0o700); e != nil {
		return e
	}
	if e := os.Chmod(dir, 0o700); e != nil {
		return e
	}
	record, e := json.Marshal(cachedDiscovery{ETag: etag, Document: b})
	if e != nil {
		return e
	}
	return writeCacheFile(filepath.Join(dir, "discovery-cache.json"), record)
}

func atomicWriteFile(path string, data []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".googleplay-mcp-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0o600); e == nil {
		_, e = f.Write(data)
	}
	if closeErr := f.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
