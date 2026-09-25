package googleplay

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestServiceAccountLoaderRejectsNonServiceAccountsAndUntrustedTokenURLs(t *testing.T) {
	dir := t.TempDir()
	for name, document := range map[string]string{
		"user": `{"type":"authorized_user","token_uri":"https://oauth2.googleapis.com/token"}`,
		"url":  `{"type":"service_account","token_uri":"https://attacker.example/token"}`,
	} {
		path := filepath.Join(dir, name+".json")
		if err := os.WriteFile(path, []byte(document), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewServiceAccountTokenSource(path); err == nil {
			t.Fatalf("%s credential accepted", name)
		}
	}
	if _, err := NewServiceAccountTokenSource(""); err == nil {
		t.Fatal("missing credential path accepted")
	}
}

type tokenFunc func() (*oauth2.Token, error)

func (f tokenFunc) Token() (*oauth2.Token, error) { return f() }

func TestServiceAccountTokenSourceCachesAndUsesInvocationContext(t *testing.T) {
	calls := 0
	source := &serviceAccountTokenSource{newSource: func(ctx context.Context) oauth2.TokenSource {
		return tokenFunc(func() (*oauth2.Token, error) {
			calls++
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &oauth2.Token{AccessToken: "cached", Expiry: time.Now().Add(time.Hour)}, nil
		})
	}}
	if got, err := source.Token(context.Background()); err != nil || got != "cached" {
		t.Fatalf("first token = %q, %v", got, err)
	}
	if got, err := source.Token(context.Background()); err != nil || got != "cached" || calls != 1 {
		t.Fatalf("cached token = %q, calls=%d, err=%v", got, calls, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Token(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context = %v", err)
	}
	source.cached = nil
	timedOut, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	if _, err := source.Token(timedOut); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed out context = %v", err)
	}
}
