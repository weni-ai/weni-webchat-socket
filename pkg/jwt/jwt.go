package jwt

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidPrivateKey = errors.New("invalid private key")
	ErrEmptyPrivateKey   = errors.New("private key is empty")
)

// Signer is a JWT token signer
type Signer struct {
	privateKey     *rsa.PrivateKey
	expirationMins int64
}

// NewSigner creates a new JWT signer from a PEM-encoded private key
// The privateKey should be a PEM-encoded RSA private key
// expirationMins is the token expiration time in minutes
func NewSigner(privateKeyPEM string, expirationMins int64) (*Signer, error) {
	if privateKeyPEM == "" {
		return nil, ErrEmptyPrivateKey
	}

	// Handle escaped newlines from environment variables
	privateKeyPEM = strings.ReplaceAll(privateKeyPEM, "\\n", "\n")

	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, ErrInvalidPrivateKey
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, ErrInvalidPrivateKey
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return nil, ErrInvalidPrivateKey
		}
	}

	return &Signer{
		privateKey:     privateKey,
		expirationMins: expirationMins,
	}, nil
}

// GenerateToken generates a signed JWT token with optional custom claims
func (s *Signer) GenerateToken(customClaims map[string]interface{}) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Unix(),
		"exp": now.Add(time.Duration(s.expirationMins) * time.Minute).Unix(),
	}

	// Add custom claims
	for key, value := range customClaims {
		claims[key] = value
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(s.privateKey)
}

// GenerateTokenWithSubject generates a signed JWT token with a subject claim
func (s *Signer) GenerateTokenWithSubject(subject string) (string, error) {
	return s.GenerateToken(map[string]interface{}{
		"sub": subject,
	})
}

// GenerateSimpleToken generates a signed JWT token with only iat and exp claims
func (s *Signer) GenerateSimpleToken() (string, error) {
	return s.GenerateToken(nil)
}
