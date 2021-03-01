package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

var (
	ErrorConnectionClosed = errors.New("unable to send: connection closed")
	ErrorBadRequest       = errors.New("unable to send: bad request")
)

var pool = websocket.NewPool()

func setupRoutes() {
	log.Trace("Setting up routes...")
	go pool.Start()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) { serveWebsocket(pool, w, r) })
	http.HandleFunc("/send", sendHandler)
}

func serveWebsocket(pool *websocket.Pool, w http.ResponseWriter, r *http.Request) {
	log.Trace("Serving websocket...")
	log.Debugf("Host: %s", r.Host)

	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		log.Error(err)
		fmt.Fprint(w, "%+V\n", err)
	}

	client := &websocket.Client{
		Conn: conn,
		Pool: pool,
	}

	log.Trace("Registering client...")
	pool.Register <- client
	client.Read()
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	log.Trace("Receiving message...")

	payload := websocket.ExternalPayload{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(ErrorBadRequest.Error()))
		return
	}
	log.Trace("Message: %#v", payload)

	client, found := pool.Clients[payload.To]
	if !found {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(ErrorConnectionClosed.Error()))
		return
	}

	sender := websocket.Sender{
		Client:  client,
		Payload: &payload,
	}

	client.Pool.Sender <- sender

	w.WriteHeader(http.StatusAccepted)
}
