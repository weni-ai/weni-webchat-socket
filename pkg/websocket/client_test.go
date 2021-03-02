package websocket_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	gws "github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/handler"
	"github.com/ilhasoft/wwcs/pkg/websocket"
)

var ttRegister = []struct {
	TestName string
	Payload  websocket.SocketPayload
	Want     websocket.Client
	Error    error
}{
	{
		TestName: "Register",
		Payload: websocket.SocketPayload{
			Type:     "register",
			From:     "1234",
			Callback: "http://foo.bar",
		},
		Want: websocket.Client{
			ID:       "1234",
			Callback: "http://foo.bar",
		},
		Error: nil,
	},
	{
		TestName: "Register with blank from",
		Payload: websocket.SocketPayload{
			Type:     "register",
			Callback: "http://foo.bar",
		},
		Want:  websocket.Client{},
		Error: websocket.ErrorBlankFrom,
	},
	{
		TestName: "Register with blank callback",
		Payload: websocket.SocketPayload{
			Type: "register",
			From: "1234",
		},
		Want:  websocket.Client{},
		Error: websocket.ErrorBlankCallback,
	},
}

func TestRegister(t *testing.T) {
	go handler.Pool.Start()
	defer handler.Pool.Close()
	client, server := newClient(t)
	defer client.Conn.Close()
	defer server.Close()

	for _, tt := range ttRegister {
		t.Run(tt.TestName, func(t *testing.T) {
			err := client.Register(tt.Payload)
			if err != tt.Error {
				t.Errorf("invalid error: have \"%#v\", want \"%#v\"", err, tt.Error)
			}

			if client.ID != tt.Want.ID {
				t.Errorf("invalid ID: have %q, want %q", client.ID, tt.Want.ID)
			}

			if client.Callback != tt.Want.Callback {
				t.Errorf("invalid callback: have %q, want %q", client.Callback, tt.Want.Callback)
			}

			client.ID = ""
			client.Callback = ""
		})
	}
}

var ttRedirect = []struct {
	TestName           string
	Payload            websocket.SocketPayload
	Register           bool
	RedirectToFrontend bool
	RedirectToCallback bool
	Redirects          int
	Error              error
}{
	{
		TestName: "Redirect all",
		Payload: websocket.SocketPayload{
			Type: "message",
			Message: websocket.Message{
				Text: "testing",
			},
		},
		Register:           true,
		RedirectToFrontend: true,
		RedirectToCallback: true,
		Redirects:          3,
		Error:              nil,
	},
	{
		TestName: "Redirect frontend only",
		Payload: websocket.SocketPayload{
			Type: "message",
			Message: websocket.Message{
				Text: "testing",
			},
		},
		Register:           true,
		RedirectToFrontend: true,
		RedirectToCallback: false,
		Redirects:          1,
		Error:              nil,
	},
	{
		TestName: "Redirect callback only",
		Payload: websocket.SocketPayload{
			Type: "message",
			Message: websocket.Message{
				Text: "testing",
			},
		},
		Register:           true,
		RedirectToFrontend: false,
		RedirectToCallback: true,
		Redirects:          2,
		Error:              nil,
	},
	{
		TestName: "Redirect nothing",
		Payload: websocket.SocketPayload{
			Type: "message",
			Message: websocket.Message{
				Text: "testing",
			},
		},
		Register:           true,
		RedirectToFrontend: false,
		RedirectToCallback: false,
		Redirects:          0,
		Error:              websocket.ErrorNoRedirects,
	},
	{
		TestName: "Redirect without register",
		Payload: websocket.SocketPayload{
			Type: "message",
			Message: websocket.Message{
				Text: "testing",
			},
		},
		Register:           false,
		RedirectToFrontend: true,
		RedirectToCallback: true,
		Redirects:          0,
		Error:              websocket.ErrorNeedRegistration,
	},
}

func TestRedirect(t *testing.T) {
	go handler.Pool.Start()
	defer handler.Pool.Close()
	client, server := newClient(t)
	defer client.Conn.Close()
	defer server.Close()

	for _, tt := range ttRedirect {
		t.Run(tt.TestName, func(t *testing.T) {
			if tt.Register {
				tt.Payload.From = "uuid"
				tt.Payload.Callback = "https://google.com"
				err := client.Register(tt.Payload)
				if err != nil {
					t.Errorf("unable to register client: %v", err)
				}
			}

			config.Get.Websocket.RedirectToCallback = tt.RedirectToCallback
			config.Get.Websocket.RedirectToFrontend = tt.RedirectToFrontend

			redirects, err := client.Redirect(tt.Payload)
			if err != tt.Error {
				t.Errorf(`invalid error: have "%d", want "%d"`, err, tt.Error)
			}

			if redirects != tt.Redirects {
				t.Errorf(`invalid redirects: have "%d", want "%d"`, redirects, tt.Redirects)
			}

			client.ID = ""
			client.Callback = ""
		})
	}
}

func newClient(t *testing.T) (*websocket.Client, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(handler.WSHandler))
	url := fmt.Sprintf("ws%s/ws", strings.TrimPrefix(server.URL, "http"))

	ws, _, err := gws.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("could not open a ws connection on %s: %v", url, err)
	}

	client := &websocket.Client{
		Conn: ws,
		Pool: handler.Pool,
	}

	return client, server
}
