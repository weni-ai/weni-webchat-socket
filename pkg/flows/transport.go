package flows

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ilhasoft/wwcs/pkg/jwt"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const channelUUIDContextKey contextKey = "channelUUID"

// jwtTransport is a custom http.RoundTripper that automatically adds JWT authentication.
// It extracts the channelUUID from the request context and generates a JWT token.
type jwtTransport struct {
	base      http.RoundTripper
	jwtSigner *jwt.Signer
}

// newJWTTransport creates a new jwtTransport with the given signer.
// If signer is nil, requests will be made without authentication.
func newJWTTransport(signer *jwt.Signer) *jwtTransport {
	return &jwtTransport{
		base:      http.DefaultTransport,
		jwtSigner: signer,
	}
}

// RoundTrip implements the http.RoundTripper interface.
// It extracts the channelUUID from the request context and adds the JWT Authorization header.
func (t *jwtTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.jwtSigner != nil {
		if channelUUID, ok := req.Context().Value(channelUUIDContextKey).(string); ok && channelUUID != "" {
			token, err := t.jwtSigner.GenerateToken(map[string]interface{}{
				"channel_uuid": channelUUID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to generate JWT token: %w", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return t.base.RoundTrip(req)
}

// withChannelUUID returns a context with the channelUUID set for JWT authentication.
func withChannelUUID(ctx context.Context, channelUUID string) context.Context {
	return context.WithValue(ctx, channelUUIDContextKey, channelUUID)
}
