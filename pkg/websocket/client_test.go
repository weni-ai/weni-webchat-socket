package websocket

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var ttParsePayload = []struct {
	TestName string
	Payload  OutgoingPayload
	Err      error
}{
	{
		TestName: "Register Client",
		Payload: OutgoingPayload{
			Type:     "register",
			Callback: "https://foo.bar",
			From:     "00001",
		},
		Err: nil,
	},
	{
		TestName: "Send Message",
		Payload: OutgoingPayload{
			Type:     "message",
			Callback: "https://foo.bar",
			From:     "00002",
		},
		Err: ErrorBlankMessageType,
	},
	{
		TestName: "Invalid PayloadType",
		Payload:  OutgoingPayload{},
		Err:      ErrorInvalidPayloadType,
	},
}

func TestParsePayload(t *testing.T) {
	pool := NewPool()
	client := &Client{
		Conn: nil,
	}

	for _, tt := range ttParsePayload {
		t.Run(tt.TestName, func(t *testing.T) {
			client.ID = tt.Payload.From
			client.Callback = tt.Payload.Callback

			err := client.ParsePayload(pool, tt.Payload, toTest)
			if err != tt.Err {
				t.Errorf("got %v, want %v", err, tt.Err)
			}
		})
	}
}

var ttClientRegister = []struct {
	TestName string
	Payload  OutgoingPayload
	Err      error
}{
	{
		TestName: "Register Client",
		Payload: OutgoingPayload{
			From:     "00001",
			Callback: "https://foo.bar",
			Trigger:  "",
		},
		Err: nil,
	},
	{
		TestName: "Duplicated Register",
		Payload: OutgoingPayload{
			From:     "00001",
			Callback: "https://foo.bar",
			Trigger:  "",
		},
		Err: ErrorIDAlreadyExists,
	},
	{
		TestName: "Register with trigger",
		Payload: OutgoingPayload{
			From:     "00002",
			Callback: "https://foo.bar",
			Trigger:  "ok",
		},
		Err: nil,
	},
	{
		TestName: "Blank From",
		Payload: OutgoingPayload{
			From:     "",
			Callback: "https://foo.bar",
			Trigger:  "",
		},
		Err: fmt.Errorf("%v blank from", errorPrefix),
	},
	{
		TestName: "Blank Callback",
		Payload: OutgoingPayload{
			From:     "00003",
			Callback: "",
			Trigger:  "",
		},
		Err: fmt.Errorf("%v blank callback", errorPrefix),
	},
	{
		TestName: "Blank Callback",
		Payload: OutgoingPayload{
			From:     "",
			Callback: "",
			Trigger:  "",
		},
		Err: fmt.Errorf("%v blank from, blank callback", errorPrefix),
	},
}

func TestClientRegister(t *testing.T) {
	pool := NewPool()
	var poolSize int
	client := &Client{
		Conn: nil,
	}

	for _, tt := range ttClientRegister {
		t.Run(tt.TestName, func(t *testing.T) {
			client.ID = tt.Payload.From
			client.Callback = tt.Payload.Callback

			err := client.Register(pool, tt.Payload, toTest)
			if fmt.Sprint(err) != fmt.Sprint(tt.Err) {
				t.Errorf("got %v / want %v", err, tt.Err)
			}

			if err == nil {
				poolSize++
			}

			if len(pool.Clients) != poolSize {
				t.Errorf("pool size equal %d, want %d", len(pool.Clients), poolSize)
			}
		})
	}
}

func TestClientUnregister(t *testing.T) {
	client := &Client{
		ID:       "123",
		Callback: "https://foo.bar",
		Conn:     nil,
	}
	pool := &Pool{
		Clients: map[string]*Client{
			client.ID: client,
		},
	}

	client.Unregister(pool)
	if len(pool.Clients) != 0 {
		t.Errorf("pool size equal %d, want %d", len(pool.Clients), 0)
	}
}

var errorInvalidTestURL = errors.New("test url")

const invalidURL = "https://error.url"

var ttRedirect = []struct {
	TestName string
	Payload  OutgoingPayload
	Err      error
}{
	{
		TestName: "Text Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type: "text",
				Text: "hello!",
			},
		},
		Err: nil,
	},
	{
		TestName: "Image Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:     "image",
				MediaURL: "https://foo.bar/image.png",
				Caption:  "My caption",
			},
		},
		Err: nil,
	},
	{
		TestName: "Video Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:     "video",
				MediaURL: "https://foo.bar/video.mp4",
				Caption:  "My caption",
			},
		},
		Err: nil,
	},
	{
		TestName: "Audio Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:     "audio",
				MediaURL: "https://foo.bar/audio.mp3",
				Caption:  "My caption",
			},
		},
		Err: nil,
	},
	{
		TestName: "File Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:     "file",
				MediaURL: "https://foo.bar/file.pdf",
				Caption:  "My caption",
			},
		},
		Err: nil,
	},
	{
		TestName: "Location Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:      "location",
				Latitude:  "12321312",
				Longitude: "12312321",
			},
		},
		Err: nil,
	},
	{
		TestName: "Blank Text Message",
		Payload: OutgoingPayload{
			Type:     "message",
			Callback: "https://foo.bar",
			From:     "00003",
			Message: Message{
				Type: "text",
				Text: "",
			},
		},
		Err: fmt.Errorf("%v blank text", errorPrefix),
	},
	{
		TestName: "Need Registration",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "",
			Callback: "",
			Message: Message{
				Type: "text",
				Text: "hello!",
			},
		},
		Err: ErrorNeedRegistration,
	},
	{
		TestName: "Request Error",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: invalidURL,
			Message: Message{
				Type: "text",
				Text: "hello!",
			},
		},
		Err: errorInvalidTestURL,
	},
	{
		TestName: "Invalid Message type",
		Payload: OutgoingPayload{
			Type:     "message",
			Callback: "https://foo.bar",
			From:     "00003",
			Message: Message{
				Type: "foo",
			},
		},
		Err: ErrorInvalidMessageType,
	},
	{
		TestName: "Ping",
		Payload: OutgoingPayload{
			Type:     "ping",
			Callback: "https://foo.bar",
			From:     "00003",
			Message:  Message{},
		},
	},
}

func toTest(url string, data interface{}) error {
	if url == invalidURL {
		return errorInvalidTestURL
	}

	return nil
}

func TestRedirect(t *testing.T) {
	c, ws, s := newTestClient(t)
	defer c.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tt := range ttRedirect {
		t.Run(tt.TestName, func(t *testing.T) {
			c.ID = tt.Payload.From
			c.Callback = tt.Payload.Callback

			err := c.Redirect(tt.Payload, toTest)
			if fmt.Sprint(err) != fmt.Sprint(tt.Err) {
				t.Errorf("got \"%v\", want: \"%v\"", err, tt.Err)
			}
		})
	}
}

var ttSend = []struct {
	TestName string
	Payload  IncomingPayload
	Want     string
	Err      error
}{
	{
		TestName: "Text Message",
		Payload: IncomingPayload{
			Type: "message",
			To:   "1232",
			From: "Caio",
			Message: Message{
				Type: "text",
				Text: "hello!",
			},
		},
		Want: fmt.Sprintln(`{"type":"message","to":"1232","from":"Caio","message":{"type":"text","timestamp":"","text":"hello!"}}`),
		Err:  nil,
	},
	{
		TestName: "Pong Message",
		Payload: IncomingPayload{
			Type: "pong",
		},
		Want: fmt.Sprintln(`{"type":"pong","to":"","from":"","message":{"type":"","timestamp":""}}`),
		Err:  nil,
	},
}

func TestSend(t *testing.T) {
	c, ws, s := newTestClient(t)
	defer c.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tt := range ttSend {
		t.Run(tt.TestName, func(t *testing.T) {
			c.ID = tt.Payload.From

			err := c.Send(tt.Payload)
			if err != tt.Err {
				t.Errorf("got %v, want: %v", err, tt.Err)
			}
			assertReceiveMessage(t, ws, tt.Want)
		})
	}
}

func assertReceiveMessage(t *testing.T, ws *websocket.Conn, message string) {
	if err := ws.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	_, p, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if string(p) != message {
		t.Fatalf("different received message\ngot:\t%v\nwant:\t%v", string(p), message)
	}
}

func newTestClient(t *testing.T) (*Client, *websocket.Conn, *httptest.Server) {
	t.Helper()
	server, ws, conn := newTestServer(t)

	client := &Client{
		Conn: conn,
	}

	return client, ws, server
}
