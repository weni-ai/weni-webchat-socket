package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

var pool = websocket.NewPool()

func setupRoutes() *mux.Router {
	log.Trace("Setting up routes...")
	r := mux.NewRouter()
	go pool.Start()

	log.Trace("Handling /ws/{id}")
	r.HandleFunc("/ws/{id}", func(w http.ResponseWriter, r *http.Request) { serveWebsocket(pool, w, r) })
	log.Trace("Handling /send")
	r.HandleFunc("/send", sendHandler)

	return r
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	log.Trace("Receiving message...")

	payload := websocket.ExternalPayload{}
	err := json.NewDecoder(r.Body).Decode(&payload)
	if err != nil {
		log.Error(err)
		return
	}
	log.Trace("Message: %#v", payload)

	client, found := pool.Clients[payload.To]
	if !found {
		log.Errorf("Connection %s is closed", payload.To)
		return
	}

	sender := websocket.Sender{
		Client:  client,
		Payload: &payload,
	}

	client.Pool.Sender <- sender
}

func serveWebsocket(pool *websocket.Pool, w http.ResponseWriter, r *http.Request) {
	log.Trace("Serving websocket...")
	vars := mux.Vars(r)
	clientID, found := vars["id"]
	if !found {
		return
	}

	log.Debugf("Host: %s", r.Host)

	conn, err := websocket.Upgrade(w, r)
	if err != nil {
		log.Error(err)
		fmt.Fprint(w, "%+V\n", err)
	}

	client := &websocket.Client{
		ID:   clientID,
		Conn: conn,
		Pool: pool,
	}

	log.Trace("Registering client...")
	pool.Register <- client
	client.Read()
}
