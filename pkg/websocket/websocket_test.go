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

func newTestServer(t *testing.T) (*httptest.Server, *websocket.Conn, *websocket.Conn) {
	t.Helper()
	ready := make(chan struct{})
	var ws *websocket.Conn
	var upgradeErr error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, upgradeErr = Upgrade(w, r)
		close(ready)
		if upgradeErr != nil {
			t.Errorf("unable to handle endpoint: %v", upgradeErr)
		}
	}))

	url := fmt.Sprintf("ws%s/ws", strings.TrimPrefix(server.URL, "http"))
	conn := newTestWSConn(t, url)
	<-ready

	if upgradeErr != nil {
		t.Fatalf("server-side websocket upgrade failed: %v", upgradeErr)
	}

	return server, ws, conn
}
