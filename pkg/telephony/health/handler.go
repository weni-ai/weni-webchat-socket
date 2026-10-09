package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

// Handler serves GET /healthcheck for the telephony HTTP server.
type Handler struct {
	RDB     *redis.Client
	Timeout time.Duration
}

// Status is the JSON payload returned by the health check endpoint.
type Status struct {
	Redis string `json:"redis,omitempty"`
}

// ServeHTTP verifies telephony runtime dependencies without side effects.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := Status{Redis: "ok"}
	hasError := false

	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if err := h.RDB.Ping(ctx).Err(); err != nil {
		status.Redis = err.Error()
		hasError = true
		log.WithError(err).Warn("telephony health check: redis ping failed")
	}

	w.Header().Set("Content-Type", "application/json")
	if hasError {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	if err := json.NewEncoder(w).Encode(status); err != nil {
		log.WithError(err).Error("telephony health check: failed to encode response")
	}
}
