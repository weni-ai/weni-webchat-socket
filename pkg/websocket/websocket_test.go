package websocket

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestUpgrade(t *testing.T) {
	s, ws, conn := newTestServer(t)
	defer conn.Close()
	defer ws.Close()
	defer s.Close()
}

func newTestWSConn(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("could not open a ws connection on %s: %v", url, err)
	}

	return conn
}

// TODO: need race fix
func newTestServer(t *testing.T) (*httptest.Server, *websocket.Conn, *websocket.Conn) {
	t.Helper()
	var ws *websocket.Conn
	var err error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err = Upgrade(w, r)
		if err != nil {
			t.Fatalf("unable to handle endpoint: %v", err)
		}
	}))

	url := fmt.Sprintf("ws%s/ws", strings.TrimPrefix(server.URL, "http"))
	conn := newTestWSConn(t, url)

	return server, ws, conn
}
