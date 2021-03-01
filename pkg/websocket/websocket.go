package websocket

import (
	"net/http"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1 << 10,
	WriteBufferSize: 1 << 10,
	CheckOrigin:     checkOrigin,
}

// checkOrigin will check the origin of our connection this will
// allow us to make requests from our frontend server to here.
func checkOrigin(r *http.Request) bool {
	log.Trace("Checking Origin...")
	return true
}

// Upgrade our http connection to a websocket connection
func Upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	log.Trace("Upgrading connection...")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(err)
		return nil, err
	}

	return conn, nil
}
