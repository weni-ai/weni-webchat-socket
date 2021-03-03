package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWebsocket(t *testing.T) {
	t.Run("Receive message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(WSHandler))
		url := fmt.Sprintf("ws%s/ws", strings.TrimPrefix(server.URL, "http"))
		ws, _, err := websocket.DefaultDialer.Dial(url, nil)

		if err != nil {
			t.Fatalf("could not open a ws connection on %s: %v", url, err)
		}
		defer ws.Close()
		defer server.Close()
	})
}
