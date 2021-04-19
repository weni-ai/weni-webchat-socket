package handler

import (
	"net/http"
)

// HealthCheckHandler is used to provide a mechanism to check the service status
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
