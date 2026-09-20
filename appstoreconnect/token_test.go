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
