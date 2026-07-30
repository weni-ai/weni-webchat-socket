package audiosocket

import (
	"encoding/json"
	"errors"
	"net/http"
)

// RegistrationHandler serves POST /telephony/sessions.
type RegistrationHandler struct {
	Registrar       SessionRegistrar
	AudioSocketAddr string
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

// ServeHTTP handles session registration requests from Asterisk.
func (h *RegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req registrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.DID == "" || req.Origin == "" {
		http.Error(w, "missing did or origin", http.StatusBadRequest)
		return
	}

	sessionID, err := h.Registrar.Register(req.DID, req.CallerID, req.Origin)
	if err != nil {
		if errors.Is(err, ErrChannelNotFound) {
			http.Error(w, "did not configured", http.StatusNotFound)
			return
		}
		if errors.Is(err, ErrSTTDependencyDown) {
			http.Error(w, "stt dependency unavailable", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "registration failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(registrationResponse{
		SessionID:       sessionID,
		AudioSocketAddr: h.AudioSocketAddr,
	})
}
