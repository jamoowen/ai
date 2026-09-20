package appstoreconnect

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

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
	if parsed.Header["alg"] != "ES256" || parsed.Header["kid"] != "kid" || parsed.Header["typ"] != "JWT" || claims["iss"] != "issuer" || claims["aud"] != "appstoreconnect-v1" || claims["sub"] != nil || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(5*time.Minute).Unix()) {
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
	if parsed.Header["alg"] != "ES256" || parsed.Header["kid"] != "kid" || parsed.Header["typ"] != "JWT" || claims["aud"] != "appstoreconnect-v1" || claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(5*time.Minute).Unix()) || claims["sub"] != "user" || claims["iss"] != nil {
		t.Fatalf("individual claims %#v", claims)
	}
	if _, err := NewES256TokenSource("kid", "", p, 21*time.Minute, nil); err == nil {
		t.Fatal("overlong lifetime accepted")
	}
	if _, err := NewES256TokenSource("kid", "", p, -time.Minute, nil); err == nil {
		t.Fatal("negative lifetime accepted")
	}
	defaults, err := NewES256TokenSource("kid", "", p, 0, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	raw, err = defaults.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = jwt.Parse(raw, func(token *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	claims = parsed.Claims.(jwt.MapClaims)
	if claims["exp"].(float64)-claims["iat"].(float64) != 300 {
		t.Fatal("default lifetime is not five minutes")
	}
}

func TestES256TokenSourceCachesAndRefreshesTokens(t *testing.T) {
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
	s, err := NewES256TokenSource("kid", "issuer", p, 2*time.Minute, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(89 * time.Second)
	if second, err := s.Token(context.Background()); err != nil || second != first {
		t.Fatalf("token was not reused: %q %v", second, err)
	}
	now = now.Add(time.Second)
	refreshed, err := s.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == first {
		t.Fatal("token was not refreshed at margin")
	}
	parsed, err := jwt.Parse(refreshed, func(token *jwt.Token) (any, error) { return &key.PublicKey, nil }, jwt.WithTimeFunc(func() time.Time { return now }))
	if err != nil {
		t.Fatal(err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["iat"] != float64(now.Unix()) || claims["exp"] != float64(now.Add(2*time.Minute).Unix()) {
		t.Fatalf("refreshed claims %#v", claims)
	}
	var group sync.WaitGroup
	tokens := make(chan string, 20)
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			raw, err := s.Token(context.Background())
			if err != nil {
				t.Errorf("Token: %v", err)
				return
			}
			tokens <- raw
		}()
	}
	group.Wait()
	close(tokens)
	for raw := range tokens {
		if raw != refreshed {
			t.Fatal("concurrent callers did not share cached token")
		}
	}
}
