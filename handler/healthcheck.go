package handler

import (
	"errors"
	"net/http"

	"github.com/ilhasoft/wwcs/pkg/websocket"
)

var (
	ErrorAWSConnection = errors.New("Unable to connect to AWS")
)

// HealthCheckHandler is used to provide a mechanism to check the service status
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	err := websocket.CheckAWS()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(ErrorAWSConnection.Error()))
		return
	}

	w.WriteHeader(http.StatusOK)
}
