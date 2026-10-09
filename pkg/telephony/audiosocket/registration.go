package audiosocket

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// RegistrationHandler serves POST /telephony/sessions.
type RegistrationHandler struct {
	Registrar       SessionRegistrar
	AudioSocketAddr string
	AuthToken       string
}

type registrationRequest struct {
	DID      string `json:"did"`
	CallerID string `json:"caller_id"`
	Origin   string `json:"origin"`
}

type registrationResponse struct {
	SessionID       string `json:"session_id"`
	AudioSocketAddr string `json:"audiosocket_addr"`
}

type registrationErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(registrationErrorResponse{
		Error:   code,
		Message: message,
	})
}

// ServeHTTP handles session registration requests from Asterisk.
func (h *RegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if !h.authorized(r) {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_body", "invalid request body")
		return
	}

	if req.DID == "" || req.Origin == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_did_or_origin", "missing did or origin")
		return
	}

	sessionID, err := h.Registrar.Register(req.DID, req.CallerID, req.Origin)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			writeJSONError(w, http.StatusNotFound, "did_not_configured", "did not configured")
			return
		}
		if errors.Is(err, ErrSTTDependencyDown) {
			writeJSONError(w, http.StatusServiceUnavailable, "stt_dependency_unavailable", "stt dependency unavailable")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "registration_failed", "registration failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(registrationResponse{
		SessionID:       sessionID,
		AudioSocketAddr: h.AudioSocketAddr,
	})
}

func (h *RegistrationHandler) authorized(r *http.Request) bool {
	expected := strings.TrimSpace(h.AuthToken)
	if expected == "" {
		return false
	}

	const bearerPrefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, bearerPrefix) {
		return false
	}

	token := strings.TrimSpace(auth[len(bearerPrefix):])
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}
