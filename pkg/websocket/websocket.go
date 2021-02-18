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

// // Reader will listen for new messages being sent to our websocket endpoint
// func Reader(conn *websocket.Conn) {
// 	log.Trace("Starting websocket.Reader")
// 	for {
// 		// read in a message
// 		log.Trace("Reading messages")
// 		messageType, p, err := conn.ReadMessage()
// 		if err != nil {
// 			log.Error(err)
// 			return
// 		}

// 		log.Tracef("messageType:%s message:%s", messageType, p)

// 		// send to our websocket endpoint
// 		log.Trace("Sending message to websocket endpoint")
// 		if err := conn.WriteMessage(messageType, p); err != nil {
// 			log.Error(err)
// 			return
// 		}
// 	}
// }

// // Writer ...
// func Writer(conn *websocket.Conn) {
// 	log.Trace("Starting websocket.Writer")
// 	for {
// 		log.Trace("Sending message")
// 		messageType, r, err := conn.NextReader()
// 		if err != nil {
// 			log.Error(err)
// 			return
// 		}
// 		w, err := conn.NextWriter(messageType)
// 		if err != nil {
// 			log.Error(err)
// 			return
// 		}
// 		if _, err := io.Copy(w, r); err != nil {
// 			log.Error(err)
// 			return
// 		}
// 		if err := w.Close(); err != nil {
// 			log.Error(err)
// 			return
// 		}
// 	}
// }
