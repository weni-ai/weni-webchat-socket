package handler

import (
	"fmt"
	"net/http"

	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

// WSHandler is the websocket handler
func WSHandler(w http.ResponseWriter, r *http.Request) {
	log.Trace("Serving websocket")
	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		log.Error(err)
		fmt.Fprint(w, "%+V\n", err)
	}

	client := &websocket.Client{
		Conn: conn,
		Pool: Pool,
	}

	client.Read()
}
