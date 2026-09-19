package appstoreconnect

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type ES256TokenSource struct {
	KeyID, IssuerID, PrivateKeyPath string
	Lifetime                        time.Duration
	Now                             func() time.Time
	key                             *ecdsa.PrivateKey
}

func NewES256TokenSource(keyID, issuerID, privateKeyPath string, lifetime time.Duration, now func() time.Time) (*ES256TokenSource, error) {
	if keyID == "" || privateKeyPath == "" {
		return nil, errors.New("ASC_KEY_ID and ASC_PRIVATE_KEY_PATH are required")
	}
	if lifetime == 0 {
		lifetime = 5 * time.Minute
	}
	if lifetime > 20*time.Minute || lifetime <= 0 {
		return nil, errors.New("JWT lifetime must be between zero and 20 minutes")
	}
	b, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("private key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not EC")
	}
	if now == nil {
		now = time.Now
	}
	return &ES256TokenSource{keyID, issuerID, privateKeyPath, lifetime, now, ec}, nil
}
func (s *ES256TokenSource) Token(context.Context) (string, error) {
	now := s.Now()
	claims := jwt.MapClaims{"aud": "appstoreconnect-v1", "iat": now.Unix(), "exp": now.Add(s.Lifetime).Unix()}
	if s.IssuerID != "" {
		claims["iss"] = s.IssuerID
	} else {
		claims["sub"] = "user"
	}
	t := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	t.Header["kid"] = s.KeyID
	return t.SignedString(s.key)
}
func (s *ES256TokenSource) String() string { return fmt.Sprintf("ES256TokenSource(%s)", s.KeyID) }
