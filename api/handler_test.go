package main

// import (
// 	"net/http"
// 	"net/http/httptest"
// 	"strings"
// 	"testing"

// 	"github.com/gorilla/websocket"
// )

// func TestWebsocket(t *testing.T) {
// 	t.Run("Receive message", func(t *testing.T) {
// 		Router().ser
// 		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { serveWebsocket(pool, w, r) }))
// 		ws := mustDialWS(t, "ws"+strings.TrimPrefix(server.URL, "http")+"/ws")

// 		defer ws.Close()
// 		defer server.Close()

// 		writeWSMessage(t, ws, "Hello!")
// 	})
// }

// // func assertWebsocketGotMsg(t *testing.T, ws *websocket.Conn, want string) {
// // 	_, msg, _ := ws.ReadMessage()
// // 	if string(msg) != want {
// // 		t.Errorf(`got "%s", want "%s"`, string(msg), want)
// // 	}
// // }

// func writeWSMessage(t *testing.T, conn *websocket.Conn, message string) {
// 	t.Helper()
// 	if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
// 		t.Fatalf("could not send message over ws connection: %v", err)
// 	}
// }

// func mustDialWS(t *testing.T, url string) *websocket.Conn {
// 	t.Helper()
// 	ws, _, err := websocket.DefaultDialer.Dial(url, nil)

// 	if err != nil {
// 		t.Fatalf("could not open a ws connection on %s: %v", url, err)
// 	}
// 	return ws
// }
