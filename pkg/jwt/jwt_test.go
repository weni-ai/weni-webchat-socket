package jwt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
)

// generateTestKeyPair generates a test RSA key pair in PEM format
func generateTestKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(privateKeyPEM), string(publicKeyPEM), nil
}

func TestNewSigner(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	signer, err := NewSigner(privateKeyPEM, 60)

	assert.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestNewSignerEmptyKey(t *testing.T) {
	signer, err := NewSigner("", 60)

	assert.Error(t, err)
	assert.Equal(t, ErrEmptyPrivateKey, err)
	assert.Nil(t, signer)
}

func TestNewSignerInvalidKey(t *testing.T) {
	signer, err := NewSigner("invalid-key", 60)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPrivateKey, err)
	assert.Nil(t, signer)
}

func TestNewSignerWithEscapedNewlines(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	// Simulate escaped newlines from environment variable
	escapedKey := ""
	for _, c := range privateKeyPEM {
		if c == '\n' {
			escapedKey += "\\n"
		} else {
			escapedKey += string(c)
		}
	}

	signer, err := NewSigner(escapedKey, 60)

	assert.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestGenerateToken(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	signer, err := NewSigner(privateKeyPEM, 60)
	assert.NoError(t, err)

	token, err := signer.GenerateToken(map[string]interface{}{
		"channel_uuid": "test-channel-uuid",
	})

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse the token to verify claims
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return &signer.privateKey.PublicKey, nil
	})
	assert.NoError(t, err)
	assert.True(t, parsedToken.Valid)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	assert.Equal(t, "test-channel-uuid", claims["channel_uuid"])
	assert.NotNil(t, claims["iat"])
	assert.NotNil(t, claims["exp"])
}

func TestGenerateTokenWithSubject(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	signer, err := NewSigner(privateKeyPEM, 60)
	assert.NoError(t, err)

	token, err := signer.GenerateTokenWithSubject("test-subject")

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse the token to verify claims
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return &signer.privateKey.PublicKey, nil
	})
	assert.NoError(t, err)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	assert.Equal(t, "test-subject", claims["sub"])
}

func TestGenerateSimpleToken(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	signer, err := NewSigner(privateKeyPEM, 60)
	assert.NoError(t, err)

	token, err := signer.GenerateSimpleToken()

	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse the token to verify only iat and exp claims exist
	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return &signer.privateKey.PublicKey, nil
	})
	assert.NoError(t, err)

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	assert.NotNil(t, claims["iat"])
	assert.NotNil(t, claims["exp"])
	assert.Nil(t, claims["sub"])
	assert.Nil(t, claims["channel_uuid"])
}

func TestTokenUsesRS256Algorithm(t *testing.T) {
	privateKeyPEM, _, err := generateTestKeyPair()
	assert.NoError(t, err)

	signer, err := NewSigner(privateKeyPEM, 60)
	assert.NoError(t, err)

	token, err := signer.GenerateSimpleToken()
	assert.NoError(t, err)

	parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		return &signer.privateKey.PublicKey, nil
	})
	assert.NoError(t, err)

	assert.Equal(t, "RS256", parsedToken.Method.Alg())
}
