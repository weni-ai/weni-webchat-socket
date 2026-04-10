package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/golang/mock/gomock"
	"github.com/gorilla/websocket"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/starters"
	"github.com/ilhasoft/wwcs/pkg/vtex"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/sync/semaphore"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

var redisHost = envOr("REDIS_HOST", "localhost") + ":6379"

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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
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
							SalePrice:         "2599.90",
							Image:             "https://foo.bar/image.png",
							Description:       "Smart TV 50\" description",
							Currency:          "BRL",
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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	_ = NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
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
	tdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
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
			app := NewApp(NewPool(), tdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"has_open_ticket":true}`))
	}))

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)
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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	for _, tc := range tcSetCustomField {
		t.Run(tc.TestName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			flowsClient := flows.NewClient(server.URL, nil)
			app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

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
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	flowsClient := flows.NewClient(server.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

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

// --- RequestVoiceTokens tests ---

func TestRequestVoiceTokens_NotConfigured(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	client.ID = "wwc:test-user"
	client.Callback = "https://flows.example.com/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive"

	err := client.RequestVoiceTokens(app)
	assert.NoError(t, err)

	var received map[string]interface{}
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "voice_tokens_error", received["type"])
	assert.Equal(t, "voice mode is not configured on this server", received["error"])
}

func TestRequestVoiceTokens_NotRegistered(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, nil)
	client := &Client{ID: "", Callback: ""}

	err := client.RequestVoiceTokens(app)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request voice tokens")
}

func TestSetCustomFieldParsePayload(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	flowsClient := flows.NewClient(server.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)
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

// --- PDP Starters Tests (T007-T018) ---

func startersApp(t *testing.T, svc starters.StartersService, semWeight int64) *App {
	t.Helper()
	var sem *semaphore.Weighted
	if semWeight >= 0 {
		sem = semaphore.NewWeighted(semWeight)
	}
	return &App{
		StartersService: svc,
		StartersSem:     sem,
	}
}

func TestGetPDPStarters_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).Return(
		&starters.StartersOutput{Questions: []string{"Q1?", "Q2?"}}, nil,
	)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "test-store",
			"linkText": "test-product",
		},
	}, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var received IncomingPayload
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "starters", received.Type)
	questions, ok := received.Data["questions"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, questions, 2)
}

func TestGetPDPStarters_UnregisteredClient(t *testing.T) {
	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()

	app := startersApp(t, nil, 10)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}, app)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "id and url is blank")
}

func TestGetPDPStarters_FeatureDisabled(t *testing.T) {
	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := &App{}

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}, app)
	assert.NoError(t, err)
}

func TestGetPDPStarters_MissingRequiredFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := starters.NewMockStartersService(ctrl)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	tests := []struct {
		name string
		data map[string]interface{}
	}{
		{"nil data", nil},
		{"empty account", map[string]interface{}{"account": "", "linkText": "b"}},
		{"empty linkText", map[string]interface{}{"account": "a", "linkText": ""}},
		{"missing both", map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.GetPDPStarters(OutgoingPayload{Data: tt.data}, app)
			assert.Error(t, err)
		})
	}
}

func TestGetPDPStarters_ConcurrencyLimitExceeded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := starters.NewMockStartersService(ctrl)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 0)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}, app)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "concurrency limit")
}

func TestGetPDPStarters_LambdaError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).Return(
		nil, fmt.Errorf("lambda timeout"),
	)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var received IncomingPayload
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "error", received.Type)
	assert.Contains(t, received.Error, "lambda timeout")
}

func TestGetPDPStarters_ClientDisconnectDuringGoroutine(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	started := make(chan struct{})
	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, input starters.StartersInput) (*starters.StartersOutput, error) {
			close(started)
			time.Sleep(500 * time.Millisecond)
			return &starters.StartersOutput{Questions: []string{"Q1?"}}, nil
		},
	)

	client, ws, server := newTestClient(t)
	defer server.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}, app)
	assert.NoError(t, err)

	<-started
	ws.Close()

	time.Sleep(800 * time.Millisecond)
}

func TestGetPDPStarters_DuplicateRequestDedup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	started := make(chan struct{})
	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, input starters.StartersInput) (*starters.StartersOutput, error) {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return &starters.StartersOutput{Questions: []string{"Q1?"}}, nil
		},
	).Times(1)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	payload := OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}

	err := client.GetPDPStarters(payload, app)
	assert.NoError(t, err)
	<-started

	err = client.GetPDPStarters(payload, app)
	assert.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	received := 0
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var msg IncomingPayload
		if err := ws.ReadJSON(&msg); err != nil {
			break
		}
		assert.Equal(t, "starters", msg.Type)
		received++
	}
	assert.Equal(t, 1, received, "duplicate request should be deduped, only 1 Lambda call")
}

func TestGetPDPStarters_PerClientInFlightBlocking(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	started := make(chan struct{})
	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, input starters.StartersInput) (*starters.StartersOutput, error) {
			close(started)
			time.Sleep(300 * time.Millisecond)
			return &starters.StartersOutput{Questions: []string{"Q1?"}}, nil
		},
	).Times(1)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	err := client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{"account": "a", "linkText": "product-1"},
	}, app)
	assert.NoError(t, err)
	<-started

	err = client.GetPDPStarters(OutgoingPayload{
		Data: map[string]interface{}{"account": "a", "linkText": "product-2"},
	}, app)
	assert.NoError(t, err, "second request with different product should be silently ignored, not error")

	time.Sleep(500 * time.Millisecond)

	received := 0
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var msg IncomingPayload
		if err := ws.ReadJSON(&msg); err != nil {
			break
		}
		received++
	}
	assert.Equal(t, 1, received, "only the first request should produce a response")
}

func TestGetPDPStarters_SecondRequestAfterFirstCompletes(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	callCount := 0
	var mu sync.Mutex
	mockSvc := starters.NewMockStartersService(ctrl)
	mockSvc.EXPECT().GetStarters(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, input starters.StartersInput) (*starters.StartersOutput, error) {
			mu.Lock()
			callCount++
			n := callCount
			mu.Unlock()
			return &starters.StartersOutput{Questions: []string{fmt.Sprintf("Q%d?", n)}}, nil
		},
	).Times(2)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := startersApp(t, mockSvc, 10)

	payload := OutgoingPayload{
		Data: map[string]interface{}{
			"account":  "a",
			"linkText": "b",
		},
	}

	err := client.GetPDPStarters(payload, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg1 IncomingPayload
	err = ws.ReadJSON(&msg1)
	assert.NoError(t, err)
	assert.Equal(t, "starters", msg1.Type)

	err = client.GetPDPStarters(payload, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg2 IncomingPayload
	err = ws.ReadJSON(&msg2)
	assert.NoError(t, err)
	assert.Equal(t, "starters", msg2.Type)
}

// --- Voice / ElevenLabs Tests ---

func TestRegister_VoiceEnabledFromFlows(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	rdb.Del(context.TODO(), elevenLabsKeyCachePrefix+"09bf3dee-973e-43d3-8b94-441406c4a565")

	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/internals/elevenlabs_api_key" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"api_key":"sk_test_key"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	payload := OutgoingPayload{
		Type:     "register",
		From:     "wwc:voice-test-1",
		Callback: flowsServer.URL + "/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		Data: map[string]interface{}{
			"features": map[string]interface{}{
				"voiceMode": true,
			},
		},
	}

	err := client.Register(payload, toTest, app)
	assert.NoError(t, err)

	var received map[string]interface{}
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "ready_for_message", received["type"])

	data := received["data"].(map[string]interface{})
	assert.Equal(t, true, data["voice_enabled"])
}

func TestRegister_VoiceDisabled(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	rdb.Del(context.TODO(), elevenLabsKeyCachePrefix+"09bf3dee-973e-43d3-8b94-441406c4a565")

	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/internals/elevenlabs_api_key" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"api_key":""}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	client, ws, s := newTestClient(t)
	defer client.Conn.Close()
	defer ws.Close()
	defer s.Close()

	payload := OutgoingPayload{
		Type:     "register",
		From:     "wwc:voice-test-3",
		Callback: flowsServer.URL + "/c/wwc/09bf3dee-973e-43d3-8b94-441406c4a565/receive",
		Data: map[string]interface{}{
			"features": map[string]interface{}{
				"voiceMode": true,
			},
		},
	}

	err := client.Register(payload, toTest, app)
	assert.NoError(t, err)

	var received map[string]interface{}
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "ready_for_message", received["type"])

	data := received["data"].(map[string]interface{})
	_, hasVoiceEnabled := data["voice_enabled"]
	assert.False(t, hasVoiceEnabled, "voice_enabled should not be present when voice is disabled")
}

// --- Negative cache tests for getElevenLabsAPIKey ---

func TestGetElevenLabsAPIKey_Flows404_CachesNegativeResult(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	channelUUID := "neg-cache-404-test"
	redisKey := elevenLabsKeyCachePrefix + channelUUID

	rdb.Del(context.TODO(), redisKey)

	var flowsHitCount int32
	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/internals/elevenlabs_api_key" {
			flowsHitCount++
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	result := getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "", result)
	assert.Equal(t, int32(1), flowsHitCount, "Flows should be called once")

	cached, err := rdb.Get(context.TODO(), redisKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, elevenLabsKeyNone, cached, "sentinel value should be cached in Redis")

	result = getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "", result)
	assert.Equal(t, int32(1), flowsHitCount, "Flows should NOT be called again — negative result was cached")
}

func TestGetElevenLabsAPIKey_FlowsEmptyKey_CachesNegativeResult(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	channelUUID := "neg-cache-empty-test"
	redisKey := elevenLabsKeyCachePrefix + channelUUID

	rdb.Del(context.TODO(), redisKey)

	var flowsHitCount int32
	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/internals/elevenlabs_api_key" {
			flowsHitCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"api_key":""}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	result := getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "", result)
	assert.Equal(t, int32(1), flowsHitCount)

	cached, err := rdb.Get(context.TODO(), redisKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, elevenLabsKeyNone, cached)

	result = getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "", result)
	assert.Equal(t, int32(1), flowsHitCount, "Flows should NOT be called again — empty key was cached")
}

func TestGetElevenLabsAPIKey_SentinelInRedis_ReturnsEmptyWithoutFlowsCall(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	channelUUID := "sentinel-preloaded-test"
	redisKey := elevenLabsKeyCachePrefix + channelUUID

	rdb.Set(context.TODO(), redisKey, elevenLabsKeyNone, time.Minute*5)

	var flowsHitCount int32
	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flowsHitCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"api_key":"sk_should_not_reach"}`))
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	result := getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "", result)
	assert.Equal(t, int32(0), flowsHitCount, "Flows should NOT be called when sentinel is cached")
}

func TestGetElevenLabsAPIKey_ValidKey_StillCachedCorrectly(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)
	channelUUID := "valid-key-cache-test"
	redisKey := elevenLabsKeyCachePrefix + channelUUID

	rdb.Del(context.TODO(), redisKey)

	var flowsHitCount int32
	flowsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/internals/elevenlabs_api_key" {
			flowsHitCount++
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"api_key":"sk_real_key_123"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer flowsServer.Close()

	flowsClient := flows.NewClient(flowsServer.URL, nil)
	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", flowsClient, nil)

	result := getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "sk_real_key_123", result)
	assert.Equal(t, int32(1), flowsHitCount)

	cached, err := rdb.Get(context.TODO(), redisKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "sk_real_key_123", cached, "real key should be cached as-is, not as sentinel")

	result = getElevenLabsAPIKey(app, channelUUID)
	assert.Equal(t, "sk_real_key_123", result)
	assert.Equal(t, int32(1), flowsHitCount, "Flows should NOT be called again — valid key was cached")
}

// --- AddToCart tests ---

func vtexApp(t *testing.T, vtexClient vtex.IClient) *App {
	t.Helper()
	return &App{
		VTEXClient: vtexClient,
	}
}

func TestAddToCart_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVTEX := vtex.NewMockIClient(ctrl)
	mockVTEX.EXPECT().AddOrUpdateCartItem(gomock.Any(), "teststore", "of123", "prod_1", "seller_a").Return(nil)

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := vtexApp(t, mockVTEX)

	err := client.AddToCart(OutgoingPayload{
		Data: map[string]interface{}{
			"vtex_account":  "teststore",
			"order_form_id": "of123",
			"item": map[string]interface{}{
				"id":     "prod_1",
				"seller": "seller_a",
			},
		},
	}, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var received IncomingPayload
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "cart_updated", received.Type)
	assert.Equal(t, "prod_1", received.Data["item_id"])
}

func TestAddToCart_NotRegistered(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVTEX := vtex.NewMockIClient(ctrl)
	app := vtexApp(t, mockVTEX)

	client := &Client{ID: "", Callback: ""}

	err := client.AddToCart(OutgoingPayload{}, app)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "add to cart")
}

func TestAddToCart_FeatureDisabled(t *testing.T) {
	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := &App{}

	err := client.AddToCart(OutgoingPayload{
		Data: map[string]interface{}{
			"vtex_account":  "teststore",
			"order_form_id": "of123",
			"item": map[string]interface{}{
				"id":     "prod_1",
				"seller": "seller_a",
			},
		},
	}, app)
	assert.NoError(t, err)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var received IncomingPayload
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "cart_error", received.Type)
	assert.Equal(t, "cart feature is not available", received.Error)
}

func TestAddToCart_MissingData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVTEX := vtex.NewMockIClient(ctrl)
	app := vtexApp(t, mockVTEX)

	client := &Client{ID: "test-client", Callback: "http://example.com/callback"}

	err := client.AddToCart(OutgoingPayload{Data: nil}, app)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "data is required")
}

func TestAddToCart_MissingRequiredFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVTEX := vtex.NewMockIClient(ctrl)
	app := vtexApp(t, mockVTEX)

	client := &Client{ID: "test-client", Callback: "http://example.com/callback"}

	tests := []struct {
		name string
		data map[string]interface{}
		err  string
	}{
		{
			name: "missing vtex_account",
			data: map[string]interface{}{
				"order_form_id": "of123",
				"item":          map[string]interface{}{"id": "prod_1", "seller": "seller_a"},
			},
			err: "vtex_account and order_form_id are required",
		},
		{
			name: "missing item",
			data: map[string]interface{}{
				"vtex_account":  "teststore",
				"order_form_id": "of123",
			},
			err: "item is required",
		},
		{
			name: "missing item.id",
			data: map[string]interface{}{
				"vtex_account":  "teststore",
				"order_form_id": "of123",
				"item":          map[string]interface{}{"seller": "seller_a"},
			},
			err: "item.id and item.seller are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.AddToCart(OutgoingPayload{Data: tt.data}, app)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.err)
		})
	}
}

func TestAddToCart_VTEXError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockVTEX := vtex.NewMockIClient(ctrl)
	mockVTEX.EXPECT().AddOrUpdateCartItem(gomock.Any(), "teststore", "of123", "prod_1", "seller_a").
		Return(fmt.Errorf("vtex: get order form failed with status 500"))

	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	app := vtexApp(t, mockVTEX)

	err := client.AddToCart(OutgoingPayload{
		Data: map[string]interface{}{
			"vtex_account":  "teststore",
			"order_form_id": "of123",
			"item": map[string]interface{}{
				"id":     "prod_1",
				"seller": "seller_a",
			},
		},
	}, app)
	assert.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	var received IncomingPayload
	err = ws.ReadJSON(&received)
	assert.NoError(t, err)
	assert.Equal(t, "cart_error", received.Type)
	assert.Equal(t, "failed to update cart", received.Error)
	assert.Equal(t, "prod_1", received.Data["item_id"])
}

func TestAddToCartParsePayload(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: redisHost, DB: 3})
	defer rdb.FlushAll(context.TODO())
	cm := NewClientManager(rdb, 4)

	mockVTEX := vtex.NewMockIClient(gomock.NewController(t))
	mockVTEX.EXPECT().AddOrUpdateCartItem(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	app := NewApp(NewPool(), rdb, nil, nil, nil, cm, nil, "", nil, mockVTEX)

	client, _, s := newTestClient(t)
	defer client.Conn.Close()
	defer s.Close()
	client.ID = "test-client"
	client.Callback = "http://example.com/callback"

	toTest := func(url string, data interface{}) ([]byte, error) {
		return nil, nil
	}

	err := client.ParsePayload(app, OutgoingPayload{
		Type: "add_to_cart",
		Data: map[string]interface{}{
			"vtex_account":  "teststore",
			"order_form_id": "of123",
			"item": map[string]interface{}{
				"id":     "prod_1",
				"seller": "seller_a",
			},
		},
	}, toTest)
	assert.NoError(t, err)
}
