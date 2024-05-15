package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, cm, nil)
	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tt := range ttParsePayload {
		t.Run(tt.TestName, func(t *testing.T) {
			client.ID = tt.Payload.From
			client.Callback = tt.Payload.Callback

			err := client.ParsePayload(app, tt.Payload, toTest)
			if err != tt.Err {
				t.Errorf("got %v, want %v", err, tt.Err)
			}
		})
	}
}

var ttCloseSession = []struct {
	TestName string
	Payload  OutgoingPayload
	Err      error
}{
	{
		TestName: "Close Session",
		Payload: OutgoingPayload{
			Type:     "close_session",
			Callback: "https://foo.bar",
			From:     "00005",
			Token:    "abcde",
		},
		Err: nil,
	},
	{
		TestName: "Invalid Token",
		Payload: OutgoingPayload{
			Type:     "close_session",
			Callback: "https://foo.bar",
			From:     "00005",
			Token:    "abce",
		},
		Err: ErrorInvalidToken,
	},
	{
		TestName: "Invalid Client",
		Payload: OutgoingPayload{
			Type:     "close_session",
			Callback: "https://foo.bar",
			From:     "00000",
			Token:    "abcde",
		},
		Err: ErrorInvalidClient,
	},
}

func TestCloseSession(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, cm, nil)
	conn := NewOpenConnection(t)

	client := &Client{
		ID:        "00005",
		Conn:      conn,
		AuthToken: "abcde",
	}

	defer client.Conn.Close()

	connectedCLient := ConnectedClient{
		ID:        client.ID,
		AuthToken: client.AuthToken,
		Channel:   "123",
	}

	err := app.ClientManager.AddConnectedClient(connectedCLient)
	assert.NoError(t, err)

	// Register client that will have the session closed
	app.ClientPool.Clients[client.ID] = client

	for _, tt := range ttCloseSession {
		t.Run(tt.TestName, func(t *testing.T) {
			client.ID = tt.Payload.From
			client.Callback = tt.Payload.Callback

			err := client.ParsePayload(app, tt.Payload, toTest)
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
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, cm, nil)
	var poolSize int

	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tt := range ttClientRegister {
		t.Run(tt.TestName, func(t *testing.T) {
			client.ID = tt.Payload.From
			client.Callback = tt.Payload.Callback

			err := client.Register(tt.Payload, toTest, app)
			if fmt.Sprint(err) != fmt.Sprint(tt.Err) {
				t.Errorf("got %v / want %v", err, tt.Err)
			}

			if err == nil {
				poolSize++
			}

			if len(app.ClientPool.Clients) != poolSize {
				t.Errorf("pool size equal %d, want %d", len(app.ClientPool.Clients), poolSize)
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
	pool := &ClientPool{
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

func toTest(url string, data interface{}) ([]byte, error) {
	if url == invalidURL {
		return nil, errorInvalidTestURL
	}

	testBody, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return testBody, nil
}

func TestRedirect(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, cm, nil)
	c, ws, s := newTestClient(t)
	defer c.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tt := range ttRedirect {
		t.Run(tt.TestName, func(t *testing.T) {
			c.ID = tt.Payload.From
			c.Callback = tt.Payload.Callback

			err := c.Redirect(tt.Payload, toTest, app)
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
	{
		TestName: "Token Message",
		Payload: IncomingPayload{
			Type:  "token",
			Token: "aaaaaa",
		},
		Want: fmt.Sprintln(`{"type":"token","to":"","from":"","message":{"type":"","timestamp":""},"token":"aaaaaa"}`),
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

var tcGetHistory = []struct {
	TestName           string
	Payload            OutgoingPayload
	Err                error
	ClientRegistration OutgoingPayload
	MsgsHistory        []history.MessagePayload
	MsgsHistoryError   error
}{
	{
		TestName: "Get History Success",
		Payload: OutgoingPayload{
			Type:   "get_history",
			Params: map[string]interface{}{"limit": 10, "page": 1},
		},
		Err: nil,
		ClientRegistration: OutgoingPayload{
			Type:        "register",
			Callback:    "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:        "tester:1",
			SessionType: "remote",
		},
		MsgsHistory: []history.MessagePayload{
			{
				ID:          &primitive.ObjectID{},
				ContactURN:  "tester:1",
				ChannelUUID: "a70369f6-f48f-43d0-bf21-cacf64136a18",
				Direction:   DirectionOut.String(),
				Timestamp:   time.Now().UnixNano(),
				Message: history.Message{
					Type: "text",
					Text: "Hello!",
				},
			},
		},
	},
	{
		TestName: "Get History Without the proper SessionType",
		Payload: OutgoingPayload{
			Type:   "get_history",
			Params: map[string]interface{}{"limit": 10, "page": 1},
		},
		Err: fmt.Errorf(
			"error on get history: only client with session type %s is allowed to fetch history",
			"remote",
		),
		ClientRegistration: OutgoingPayload{
			Type:        "register",
			Callback:    "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:        "tester:1",
			SessionType: "any",
		},
		MsgsHistory: nil,
	},
	{
		TestName: "Get History Without the proper limit Params",
		Payload: OutgoingPayload{
			Type:   "get_history",
			Params: map[string]interface{}{"limit": "wrong_type", "page": 1},
		},
		Err: fmt.Errorf(
			"error on get history: could not parse limit param: %s",
			"strconv.Atoi: parsing \"wrong_type\": invalid syntax",
		),
		ClientRegistration: OutgoingPayload{
			Type:        "register",
			Callback:    "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:        "tester:1",
			SessionType: "remote",
		},
		MsgsHistory: nil,
	},
	{
		TestName: "Get History Without the proper page Params",
		Payload: OutgoingPayload{
			Type:   "get_history",
			Params: map[string]interface{}{"limit": 10, "page": "wrong_type"},
		},
		Err: fmt.Errorf(
			"error on get history: could not parse page param: %s",
			"strconv.Atoi: parsing \"wrong_type\": invalid syntax",
		),
		ClientRegistration: OutgoingPayload{
			Type:        "register",
			Callback:    "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:        "tester:1",
			SessionType: "remote",
		},
		MsgsHistory: nil,
	},
	{
		TestName: "Get History With error on Service",
		Payload: OutgoingPayload{
			Type:   "get_history",
			Params: map[string]interface{}{"limit": 10, "page": 1},
		},
		Err: errors.New("error on get history, mocked error"),
		ClientRegistration: OutgoingPayload{
			Type:        "register",
			Callback:    "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:        "tester:1",
			SessionType: "remote",
		},
		MsgsHistory:      nil,
		MsgsHistoryError: errors.New("mocked error"),
	},
}

func TestGetHistory(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	_ = NewApp(NewPool(), rdb, nil, nil, cm, nil)
	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()
	os.Setenv("WWC_SESSION_TYPE_TO_STORE", "remote")

	for _, tc := range tcGetHistory {
		t.Run(tc.TestName, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := history.NewMockService(ctrl)

			client.ID = tc.ClientRegistration.From
			client.Callback = tc.ClientRegistration.Callback
			client.SessionType = tc.ClientRegistration.SessionType
			client.Histories = mockService
			client.RegistrationMoment = time.Now()

			if tc.MsgsHistory != nil {
				mockService.EXPECT().Get(
					tc.ClientRegistration.From,
					client.ChannelUUID(),
					&client.RegistrationMoment,
					tc.Payload.Params["limit"],
					tc.Payload.Params["page"],
				).Return(tc.MsgsHistory, nil)
			}

			if tc.MsgsHistoryError != nil {
				mockService.EXPECT().Get(
					tc.ClientRegistration.From,
					client.ChannelUUID(),
					&client.RegistrationMoment,
					tc.Payload.Params["limit"],
					tc.Payload.Params["page"],
				).Return(nil, tc.MsgsHistoryError)
			}

			err := client.FetchHistory(tc.Payload)
			if err != nil {
				assert.Equal(t, err.Error(), tc.Err.Error())
			}
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

func NewOpenConnection(t *testing.T) *websocket.Conn {
	t.Helper()
	_, _, conn := newTestServer(t)

	return conn
}
