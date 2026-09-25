package googleplay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const AndroidPublisherScope = "https://www.googleapis.com/auth/androidpublisher"

type (
	TokenSource interface {
		Token(context.Context) (string, error)
	}
	serviceAccountTokenSource struct {
		newSource func(context.Context) oauth2.TokenSource
		mu        sync.Mutex
		cached    *oauth2.Token
	}
)

func (s *serviceAccountTokenSource) Token(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil && s.cached.Valid() {
		return s.cached.AccessToken, nil
	}
	t, e := s.newSource(ctx).Token()
	if e != nil {
		return "", e
	}
	if t.AccessToken == "" {
		return "", errors.New("Google OAuth returned an empty access token")
	}
	s.cached = t
	return s.cached.AccessToken, nil
}

// NewServiceAccountTokenSource accepts only an explicit service account JSON file.
func NewServiceAccountTokenSource(path string) (TokenSource, error) {
	if path == "" {
		return nil, errors.New("GP_SERVICE_ACCOUNT_KEY_PATH is required")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	var identity struct {
		Type     string `json:"type"`
		TokenURI string `json:"token_uri"`
	}
	if e = json.Unmarshal(b, &identity); e != nil {
		return nil, fmt.Errorf("parse service account JSON: %w", e)
	}
	if identity.Type != "service_account" {
		return nil, errors.New("service account JSON type must be service_account")
	}
	if identity.TokenURI != "https://oauth2.googleapis.com/token" {
		return nil, errors.New("service account token_uri must be https://oauth2.googleapis.com/token")
	}
	cfg, e := google.JWTConfigFromJSON(b, AndroidPublisherScope)
	if e != nil {
		return nil, fmt.Errorf("load service account credentials: %w", e)
	}
	return &serviceAccountTokenSource{newSource: cfg.TokenSource}, nil
}
