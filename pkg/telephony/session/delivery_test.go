package session

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	grpcserver "github.com/ilhasoft/wwcs/pkg/grpc"
	"github.com/ilhasoft/wwcs/pkg/grpc/proto"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type recordingClientManager struct {
	mu      sync.Mutex
	added   []websocket.ConnectedClient
	removed []string
	store   map[string]websocket.ConnectedClient
}

func newRecordingClientManager() *recordingClientManager {
	return &recordingClientManager{store: make(map[string]websocket.ConnectedClient)}
}

func (m *recordingClientManager) GetConnectedClients() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.store))
	for id := range m.store {
		ids = append(ids, id)
	}
	return ids, nil
}

func (m *recordingClientManager) GetConnectedClient(clientID string) (*websocket.ConnectedClient, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cc, ok := m.store[clientID]; ok {
		copy := cc
		return &copy, nil
	}
	return nil, nil
}

func (m *recordingClientManager) AddConnectedClient(client websocket.ConnectedClient) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, client)
	m.store[client.ID] = client
	return nil
}

func (m *recordingClientManager) RemoveConnectedClient(clientID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, clientID)
	delete(m.store, clientID)
	return nil
}

func (m *recordingClientManager) UpdateClientTTL(string, int) (bool, error) { return true, nil }
func (m *recordingClientManager) DefaultClientTTL() int                   { return 60 }

func TestRegistrationKeyStripsSchemeLikeGRPCNormalize(t *testing.T) {
	cases := []struct {
		contactURN string
		want       string
	}{
		{"tel:+15559876543", "+15559876543"},
		{"ext:217138695938@", "217138695938@"},
		{"+15559876543", "+15559876543"},
		{"", ""},
	}
	for _, tc := range cases {
		cs := &CallSession{ContactURN: tc.contactURN}
		assert.Equal(t, tc.want, cs.RegistrationKey(), "ContactURN=%q", tc.contactURN)
	}
}

func TestPostTranscriptSuccess(t *testing.T) {
	var received struct {
		Type     string `json:"type"`
		CallerID string `json:"caller_id"`
		CallID   string `json:"call_id"`
		Origin   string `json:"origin"`
		DID      string `json:"did"`
		Message  struct {
			Type      string `json:"type"`
			Text      string `json:"text"`
			Timestamp string `json:"timestamp"`
		} `json:"message"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&received))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"urn":"tel:+15559876543"}]}`))
	}))
	defer srv.Close()

	contactURN, err := PostTranscript(srv.URL, "+15559876543", "pstn", "+15551234567", "call-1", "hello there")
	require.NoError(t, err)
	assert.Equal(t, "tel:+15559876543", contactURN)
	assert.Equal(t, "message", received.Type)
	assert.Equal(t, "+15559876543", received.CallerID)
	assert.Equal(t, "call-1", received.CallID)
	assert.Equal(t, "pstn", received.Origin)
	assert.Equal(t, "+15551234567", received.DID)
	assert.Equal(t, "text", received.Message.Type)
	assert.Equal(t, "hello there", received.Message.Text)
	assert.NotEmpty(t, received.Message.Timestamp)
}

func TestPostTranscriptCourierStandardResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"type":"msg","urn":"tel:+15559876543"}]}`))
	}))
	defer srv.Close()

	contactURN, err := PostTranscript(srv.URL, "+15559876543", "pstn", "+15551234567", "call-1", "hello")
	require.NoError(t, err)
	assert.Equal(t, "tel:+15559876543", contactURN)
}

func TestPostTranscriptWithheldURNFromCourier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"type":"msg","urn":"tel:withheld-call-1"}]}`))
	}))
	defer srv.Close()

	contactURN, err := PostTranscript(srv.URL, "", "pstn", "+15551234567", "call-1", "hello")
	require.NoError(t, err)
	assert.Equal(t, "tel:withheld-call-1", contactURN)
}

func TestPostTranscriptNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream error", http.StatusBadGateway)
	}))
	defer srv.Close()

	_, err := PostTranscript(srv.URL, "+15559876543", "pstn", "+15551234567", "call-1", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestPostTranscriptNetworkErrorRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			hj, ok := w.(http.Hijacker)
			require.True(t, ok)
			conn, _, err := hj.Hijack()
			require.NoError(t, err)
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"urn":"tel:+15559876543"}]}`))
	}))
	defer srv.Close()

	contactURN, err := PostTranscript(srv.URL, "+15559876543", "pstn", "+15551234567", "call-1", "retry me")
	require.NoError(t, err)
	assert.Equal(t, "tel:+15559876543", contactURN)
	assert.Equal(t, int32(3), atomic.LoadInt32(&attempts))
}

func TestPostTranscriptMissingURN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, err := PostTranscript(srv.URL, "+15559876543", "pstn", "+15551234567", "call-1", "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing contact urn")
}

func TestRegisterAndDeregisterDelivery(t *testing.T) {
	cm := newRecordingClientManager()
	cs := &CallSession{
		ID:          "sess-reg",
		ChannelUUID: "ch-1",
		ContactURN:  "tel:+15559876543",
	}

	require.NoError(t, RegisterDelivery(cs, cm, "telephony-pod-1"))
	require.Len(t, cm.added, 1)
	assert.Equal(t, "+15559876543", cm.added[0].ID)
	assert.Equal(t, "ch-1", cm.added[0].Channel)
	assert.Equal(t, "telephony-pod-1", cm.added[0].PodID)
	assert.True(t, cs.deliveryRegistered)

	require.NoError(t, RegisterDelivery(cs, cm, "telephony-pod-1"))
	assert.Len(t, cm.added, 1, "second register should be idempotent")

	require.NoError(t, DeregisterDelivery(cs, cm))
	assert.Equal(t, []string{"+15559876543"}, cm.removed)
	assert.False(t, cs.deliveryRegistered)
}

func TestDeliveryCoordinatorOnCommittedTranscript(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"Message Accepted","data":[{"urn":"tel:+15559876543"}]}`))
	}))
	defer srv.Close()

	cm := newRecordingClientManager()
	mgr := NewSessionManager(nil, 10, "", nil, nil)
	cs := &CallSession{
		ID:          "sess-coord",
		CallerID:    "+15559876543",
		Origin:      "pstn",
		DID:         "+15551234567",
		ChannelUUID: "ch-1",
		State:       StateListening,
	}
	mgr.mu.Lock()
	mgr.byID[cs.ID] = cs
	mgr.mu.Unlock()

	coord := NewDeliveryCoordinator(cm, mgr, "telephony-pod-1", srv.URL)
	coord.OnCommittedTranscript(cs, &Turn{CommittedText: "order status"})

	assert.Equal(t, "tel:+15559876543", cs.ContactURN)
	assert.Equal(t, StateProcessing, cs.CurrentState())
	require.Len(t, cm.added, 1)
	assert.Equal(t, "+15559876543", cm.added[0].ID)
}

func TestGRPCStreamDeliveryToCallSession(t *testing.T) {
	clientID := "+15559876543"
	cm := newRecordingClientManager()

	cs := &CallSession{
		ID:          "sess-grpc",
		ChannelUUID: "ch-1",
		ContactURN:  "tel:+15559876543",
		State:       StateProcessing,
		Conn:        &mockAudioConn{},
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return &mockTTSClient{audio: []byte{0, 0x01, 0x02}}
		},
	}

	mgr := NewSessionManager(nil, 10, "", nil, nil)
	mgr.mu.Lock()
	mgr.byID[cs.ID] = cs
	mgr.byRegKey[clientID] = cs
	mgr.mu.Unlock()

	require.NoError(t, RegisterDelivery(cs, cm, "telephony-test-pod"))

	deliver := TelephonyDeliverFunc(mgr)
	app := &testMessageStreamApp{
		router:        &syncDeliverRouter{deliver: deliver},
		clientManager: cm,
	}
	grpcSrv := grpcserver.NewServer(app)

	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	proto.RegisterMessageStreamServiceServer(s, grpcSrv)
	go func() { _ = s.Serve(lis) }()
	defer s.Stop()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	client := proto.NewMessageStreamServiceClient(conn)
	stream, err := client.StreamMessages(context.Background())
	require.NoError(t, err)

	msgID := "msg-stream-1"
	contactURN := "tel:+15559876543"
	for _, msg := range []*proto.StreamMessage{
		{Type: "setup", MsgId: msgID, ContactUrn: contactURN, ChannelUuid: "ch-1"},
		{Type: "delta", MsgId: msgID, Content: "Hello ", ContactUrn: contactURN, ChannelUuid: "ch-1"},
		{Type: "completed", MsgId: msgID, Content: "Hello world", ContactUrn: contactURN, ChannelUuid: "ch-1"},
	} {
		require.NoError(t, stream.Send(msg))
		resp, err := stream.Recv()
		require.NoError(t, err)
		assert.Equal(t, "success", resp.Status, "msg type=%s", msg.Type)
	}

	assert.Equal(t, msgID, cs.CurrentTurn.MsgID)

	deadline := time.After(3 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for TTS playback, state=%s", cs.CurrentState())
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Greater(t, cs.Conn.(*mockAudioConn).WrittenLen(), 0)
}

type syncDeliverRouter struct {
	deliver streams.DeliverFunc
}

func (r *syncDeliverRouter) Start(context.Context) {}
func (r *syncDeliverRouter) Stop(context.Context)  {}
func (r *syncDeliverRouter) PublishToClient(_ context.Context, to string, payload []byte) error {
	return r.deliver(to, payload)
}

type testMessageStreamApp struct {
	router        streams.Router
	clientManager websocket.ClientManager
}

func (a *testMessageStreamApp) Router() streams.Router               { return a.router }
func (a *testMessageStreamApp) Histories() history.Service           { return nil }
func (a *testMessageStreamApp) ClientManager() websocket.ClientManager { return a.clientManager }
