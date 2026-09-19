package appstoreconnect

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultSpecPath = "api/apple/app-store-connect.openapi.json"

var sourceHTTPClient = &http.Client{Timeout: 20 * time.Second}

func LoadCatalogSource(source string) (*Catalog, error) {
	if source == "" {
		source = DefaultSpecPath
	}
	u, err := url.Parse(source)
	if err == nil && u.Scheme != "" {
		if u.Scheme != "https" {
			return nil, fmt.Errorf("OpenAPI URL must use HTTPS")
		}
		client := *sourceHTTPClient
		prior := client.CheckRedirect
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("OpenAPI redirects must use HTTPS")
			}
			if prior != nil {
				return prior(req, via)
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		}
		resp, err := client.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("download OpenAPI spec: HTTP %d", resp.StatusCode)
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
		if err != nil {
			return nil, err
		}
		if len(b) > 16<<20 {
			return nil, fmt.Errorf("OpenAPI spec exceeds 16 MiB")
		}
		return LoadCatalog(b)
	}
	if strings.HasPrefix(source, "//") {
		return nil, fmt.Errorf("OpenAPI URL must use HTTPS")
	}
	return LoadCatalogFile(source)
}
