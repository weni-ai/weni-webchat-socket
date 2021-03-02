package handler

import (
	"net/http"

	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

// Pool store all clients
var Pool = websocket.NewPool()

// SetupRoutes handle all routes
func SetupRoutes() {
	log.Trace("Setting up routes")
	go Pool.Start()

	http.HandleFunc("/ws", WSHandler)
	http.HandleFunc("/send", SendHandler)
}
