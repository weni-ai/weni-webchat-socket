package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/pkg/flows"
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
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil)
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
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil)
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
		// When original handler is considered dead (no heartbeat mapping),
		// registration should return the specific error and not proceed.
		Err: ErrorOriginalHandlerDead,
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
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil)
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
		TestName: "Text Message With Context",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Context:  "Context",
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
	{
		TestName: "Order Message",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type: "order",
				Order: &history.Order{
					Text: "Order placed",
					ProductItems: []history.ProductItem{
						{
							ProductRetailerID: "product-001",
							Name:              "Smart TV 50\"",
							Price:             "2999.90",
							Image:             "https://example.com/tv.jpg",
							Description:       "Smart TV 4K 50 inches",
							SellerID:          "seller-001",
							Quantity:          2,
						},
					},
				},
			},
		},
		Err: nil,
	},
	{
		TestName: "Order Message - Blank Order",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type:  "order",
				Order: nil,
			},
		},
		Err: fmt.Errorf("%v blank order", errorPrefix),
	},
	{
		TestName: "Order Message - Empty Product Items",
		Payload: OutgoingPayload{
			Type:     "message",
			From:     "Caio",
			Callback: "https://foo.bar",
			Message: Message{
				Type: "order",
				Order: &history.Order{
					Text:         "Order placed",
					ProductItems: []history.ProductItem{},
				},
			},
		},
		Err: fmt.Errorf("%v empty product_items in order", errorPrefix),
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
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil)
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
		Want: fmt.Sprintln(`{"type":"message","to":"1232","from":"Caio","message":{"type":"text","timestamp":"","text":"hello!","list_message":{"button_text":"","list_items":null}}}`),
		Err:  nil,
	},
	{
		TestName: "Pong Message",
		Payload: IncomingPayload{
			Type: "pong",
		},
		Want: fmt.Sprintln(`{"type":"pong","to":"","from":"","message":{"type":"","timestamp":"","list_message":{"button_text":"","list_items":null}}}`),
		Err:  nil,
	},
	{
		TestName: "Token Message",
		Payload: IncomingPayload{
			Type:  "token",
			Token: "aaaaaa",
		},
		Want: fmt.Sprintln(`{"type":"token","to":"","from":"","message":{"type":"","timestamp":"","list_message":{"button_text":"","list_items":null}},"token":"aaaaaa"}`),
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
			Type:     "register",
			Callback: "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:     "tester:1",
		},
		MsgsHistory: []history.MessagePayload{
			{
				ID:          &primitive.ObjectID{},
				ContactURN:  "tester:1",
				ChannelUUID: "a70369f6-f48f-43d0-bf21-cacf64136a18",
				Direction:   DirectionOut.String(),
				Timestamp:   time.Now().Unix(),
				Message: history.Message{
					Type: "text",
					Text: "Hello!",
				},
			},
		},
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
			Type:     "register",
			Callback: "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:     "tester:1",
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
			Type:     "register",
			Callback: "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:     "tester:1",
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
			Type:     "register",
			Callback: "https://foo.bar/a70369f6-f48f-43d0-bf21-cacf64136a18",
			From:     "tester:1",
		},
		MsgsHistory:      nil,
		MsgsHistoryError: errors.New("mocked error"),
	},
}

func TestGetHistory(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	_ = NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil)
	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tc := range tcGetHistory {
		t.Run(tc.TestName, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := history.NewMockService(ctrl)

			client.ID = tc.ClientRegistration.From
			client.Callback = tc.ClientRegistration.Callback
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

func TestOriginToDomain(t *testing.T) {
	origin := "http://foo.bar"
	domain, err := OriginToDomain(origin)
	assert.Nil(t, err)
	assert.Equal(t, "foo.bar", domain)
}

func TestCheckAllowedDomain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[\"domain1.com\", \"domain2.com\"]"))
	}))
	defer server.Close()

	client := flows.NewClient(server.URL, nil)

	app := &App{FlowsClient: client}

	allowed := CheckAllowedDomain(app, "09bf3dee-973e-43d3-8b94-441406c4a565", "domain1.com")

	assert.True(t, allowed)
}

var tcVerifyContactTimeout = []struct {
	TestName  string
	Payload   OutgoingPayload
	Err       error
	HasTicket bool
}{
	{
		TestName: "Verify Contact Timeout",
		Payload: OutgoingPayload{
			Type:     "verify_contact_timeout",
			From:     "wwc:1234567890",
			Callback: "https://foo.bar",
		},
		HasTicket: false,
		Err:       nil,
	},
	{
		TestName: "Verify Contact Timeout False",
		Payload: OutgoingPayload{
			Type:     "verify_contact_timeout",
			From:     "wwc:1234567890",
			Callback: "https://foo.bar",
		},
		HasTicket: true,
		Err:       nil,
	},
	{
		TestName: "Verify Contact Timeout Error",
		Payload: OutgoingPayload{
			Type:     "verify_contact_timeout",
			From:     "wwc:1234567890",
			Callback: "https://foo.bar",
		},
		HasTicket: false,
		Err:       errors.New("verify contact timeout: failed to get contact has open ticket, status code: 500"),
	},
}

func TestVerifyContactTimeout(t *testing.T) {
	tdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer tdb.FlushAll(context.TODO())
	cm := NewClientManager(tdb, 4)
	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	for _, tc := range tcVerifyContactTimeout {
		t.Run(tc.TestName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.Err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(tc.Err.Error()))
					return
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"has_open_ticket":` + strconv.FormatBool(tc.HasTicket) + `}`))
			}))
			defer server.Close()

			flowsClient := flows.NewClient(server.URL, nil)
			app := NewApp(NewPool(), tdb, nil, nil, nil, cm, nil, "", flowsClient)

			client.ID = tc.Payload.From
			client.Callback = tc.Payload.Callback

			err := client.VerifyContactTimeout(app)
			if err != nil {
				assert.Equal(t, err.Error(), tc.Err.Error())
				return
			}

			if !tc.HasTicket {
				assertReceiveMessage(t, ws, fmt.Sprintln(`{"type":"allow_contact_timeout","to":"","from":"","message":{"type":"","timestamp":"","list_message":{"button_text":"","list_items":null}}}`))
			}
		})
	}
}

func TestVerifyContactTimeoutOnParsePayload(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"has_open_ticket":true}`))
	}))

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient)
	conn := NewOpenConnection(t)

	client := &Client{
		ID:        "wwc:1234567890",
		Conn:      conn,
		AuthToken: "abcde",
		Callback:  "https://foo.bar",
	}

	defer client.Conn.Close()

	connectedCLient := ConnectedClient{
		ID:        client.ID,
		AuthToken: client.AuthToken,
		Channel:   "123",
	}

	err := app.ClientManager.AddConnectedClient(connectedCLient)
	assert.NoError(t, err)

	app.ClientPool.Clients[client.ID] = client

	err = client.ParsePayload(app, OutgoingPayload{
		Type:     "verify_contact_timeout",
		From:     "wwc:1234567890",
		Callback: "https://foo.bar",
	}, toTest)
	assert.NoError(t, err)
}

var tcSetCustomField = []struct {
	TestName    string
	Payload     OutgoingPayload
	ClientID    string
	Callback    string
	ExpectedErr string
}{
	{
		TestName: "Set Custom Field Success",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key":   "User ID",
				"value": "12345",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "",
	},
	{
		TestName: "Set Custom Field - Data is nil",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: nil,
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "set custom field: data is required",
	},
	{
		TestName: "Set Custom Field - Key is empty",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key":   "",
				"value": "12345",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "set custom field: key is required",
	},
	{
		TestName: "Set Custom Field - Key is missing",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"value": "12345",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "set custom field: key is required",
	},
	{
		TestName: "Set Custom Field - Value is empty",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key":   "User ID",
				"value": "",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "set custom field: value is required",
	},
	{
		TestName: "Set Custom Field - Value is missing",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key": "User ID",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		ExpectedErr: "set custom field: value is required",
	},
	{
		TestName: "Set Custom Field - Not Registered",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key":   "User ID",
				"value": "12345",
			},
		},
		ClientID:    "",
		Callback:    "",
		ExpectedErr: "set custom field: unable to redirect: id and url is blank",
	},
	{
		TestName: "Set Custom Field - ChannelUUID is empty",
		Payload: OutgoingPayload{
			Type: "set_custom_field",
			Data: map[string]interface{}{
				"key":   "User ID",
				"value": "12345",
			},
		},
		ClientID:    "wwc:1234567890",
		Callback:    "https://flows.example.com/invalid-callback",
		ExpectedErr: "set custom field: channelUUID is not set",
	},
}

func TestSetCustomField(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	for _, tc := range tcSetCustomField {
		t.Run(tc.TestName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			flowsClient := flows.NewClient(server.URL, nil)
			app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient)

			client := &Client{
				ID:       tc.ClientID,
				Callback: tc.Callback,
			}

			err := client.SetCustomField(tc.Payload, app)

			if tc.ExpectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.ExpectedErr)
			}
		})
	}
}

func TestSetCustomFieldAPIError(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	flowsClient := flows.NewClient(server.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient)

	client := &Client{
		ID:       "wwc:1234567890",
		Callback: "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
	}

	err := client.SetCustomField(OutgoingPayload{
		Type: "set_custom_field",
		Data: map[string]interface{}{
			"key":   "User ID",
			"value": "12345",
		},
	}, app)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "set custom field")
}

func TestSetCustomFieldParsePayload(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379", DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	flowsClient := flows.NewClient(server.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient)
	conn := NewOpenConnection(t)

	client := &Client{
		ID:        "wwc:1234567890",
		Conn:      conn,
		AuthToken: "abcde",
		Callback:  "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
	}

	defer client.Conn.Close()

	connectedCLient := ConnectedClient{
		ID:        client.ID,
		AuthToken: client.AuthToken,
		Channel:   "123",
	}

	err := app.ClientManager.AddConnectedClient(connectedCLient)
	assert.NoError(t, err)

	app.ClientPool.Clients[client.ID] = client

	toTest := func(url string, data interface{}) ([]byte, error) {
		return nil, nil
	}

	// Test that ParsePayload correctly routes to SetCustomField
	err = client.ParsePayload(app, OutgoingPayload{
		Type: "set_custom_field",
		Data: map[string]interface{}{
			"key":   "User ID",
			"value": "12345",
		},
	}, toTest)
	assert.NoError(t, err)
}
