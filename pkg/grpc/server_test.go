package grpc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ilhasoft/wwcs/pkg/grpc/proto"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRationalePublishesStreamRationale(t *testing.T) {
	router := &recordingRouter{}
	server := newRationaleTestServer(router, &recordingHistory{})

	resp, err := server.processStreamMessageWithSeq(context.Background(), &proto.StreamMessage{
		Type:       "rationale",
		MsgId:      "msg-1",
		Content:    "  Vou consultar o status do pedido informado.  ",
		ContactUrn: "ext:217138695938@",
		Metadata:   map[string]string{"rationale_index": "1"},
	}, map[string]int64{})
	require.NoError(t, err)
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "msg-1", resp.MsgId)
	assert.Equal(t, "rationale forwarded", resp.Message)

	require.Len(t, router.published, 1)
	assert.Equal(t, "217138695938@", router.published[0].to)

	var payload websocket.StreamRationalePayload
	require.NoError(t, json.Unmarshal(router.published[0].payload, &payload))
	assert.Equal(t, websocket.StreamRationalePayload{
		Type:    "stream_rationale",
		ID:      "msg-1",
		Content: "Vou consultar o status do pedido informado.",
		Index:   1,
	}, payload)
}

func TestRationaleDoesNotSaveHistory(t *testing.T) {
	histories := &recordingHistory{}
	server := newRationaleTestServer(&recordingRouter{}, histories)

	_, err := server.processStreamMessageWithSeq(context.Background(), &proto.StreamMessage{
		Type:       "rationale",
		MsgId:      "msg-1",
		Content:    "Vou consultar o status do pedido.",
		ContactUrn: "ext:217138695938@",
		Metadata:   map[string]string{"rationale_index": "1"},
	}, map[string]int64{})
	require.NoError(t, err)
	assert.Empty(t, histories.saves)
}

func TestRationaleDoesNotConsumeStreamingSequence(t *testing.T) {
	router := &recordingRouter{}
	server := newRationaleTestServer(router, &recordingHistory{})
	seq := map[string]int64{}
	ctx := context.Background()

	_, err := server.processStreamMessageWithSeq(ctx, streamMessage("setup", "msg-1", ""), seq)
	require.NoError(t, err)
	_, err = server.processStreamMessageWithSeq(ctx, rationaleMessage("msg-1", "Vou consultar.", "1"), seq)
	require.NoError(t, err)
	assert.Equal(t, int64(0), seq["msg-1"])

	resp, err := server.processStreamMessageWithSeq(ctx, streamMessage("delta", "msg-1", "Olá"), seq)
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Sequence)
	assert.Equal(t, int64(1), seq["msg-1"])

	var delta websocket.StreamDeltaPayload
	require.NoError(t, json.Unmarshal(router.published[len(router.published)-1].payload, &delta))
	assert.Equal(t, int64(1), delta.Seq)
	assert.Equal(t, "Olá", delta.V)
}

func TestRationaleDoesNotConsumeUnarySequence(t *testing.T) {
	router := &recordingRouter{}
	server := newRationaleTestServer(router, &recordingHistory{})
	ctx := context.Background()

	_, err := server.processStreamMessageWithUnarySeq(ctx, streamMessage("setup", "msg-1", ""))
	require.NoError(t, err)
	_, err = server.processStreamMessageWithUnarySeq(ctx, rationaleMessage("msg-1", "Vou consultar.", "1"))
	require.NoError(t, err)

	resp, err := server.processStreamMessageWithUnarySeq(ctx, streamMessage("delta", "msg-1", "Olá"))
	require.NoError(t, err)
	assert.Equal(t, int32(1), resp.Sequence)

	var delta websocket.StreamDeltaPayload
	require.NoError(t, json.Unmarshal(router.published[len(router.published)-1].payload, &delta))
	assert.Equal(t, int64(1), delta.Seq)
	assert.Equal(t, "Olá", delta.V)
}

func TestOfflineClientStillAcknowledgesRationale(t *testing.T) {
	router := &recordingRouter{}
	server := NewServer(&fakeMessageStreamApp{
		router:  router,
		history: &recordingHistory{},
		clients: connectedClientManager{offline: true},
	})

	resp, err := server.processStreamMessageWithSeq(context.Background(), rationaleMessage("msg-1", "Vou consultar.", "1"), map[string]int64{})
	require.NoError(t, err)
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "rationale forwarded", resp.Message)
	assert.Empty(t, router.published)
}

func TestBlankRationalePublishesNothing(t *testing.T) {
	router := &recordingRouter{}
	server := newRationaleTestServer(router, &recordingHistory{})

	resp, err := server.processStreamMessageWithSeq(context.Background(), &proto.StreamMessage{
		Type:       "rationale",
		MsgId:      "msg-1",
		Content:    "   ",
		ContactUrn: "ext:217138695938@",
	}, map[string]int64{})
	require.NoError(t, err)
	assert.Equal(t, "success", resp.Status)
	assert.Empty(t, router.published)
}

func TestRationaleOmitsInvalidOrMissingIndex(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
	}{
		{name: "invalid", metadata: map[string]string{"rationale_index": "abc"}},
		{name: "missing", metadata: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := &recordingRouter{}
			server := newRationaleTestServer(router, &recordingHistory{})

			_, err := server.processStreamMessageWithSeq(context.Background(), &proto.StreamMessage{
				Type:       "rationale",
				MsgId:      "msg-1",
				Content:    "Vou consultar.",
				ContactUrn: "ext:217138695938@",
				Metadata:   tt.metadata,
			}, map[string]int64{})
			require.NoError(t, err)
			require.Len(t, router.published, 1)
			assert.NotContains(t, string(router.published[0].payload), `"index"`)

			var payload websocket.StreamRationalePayload
			require.NoError(t, json.Unmarshal(router.published[0].payload, &payload))
			assert.Equal(t, "stream_rationale", payload.Type)
			assert.Equal(t, "msg-1", payload.ID)
			assert.Equal(t, "Vou consultar.", payload.Content)
			assert.Zero(t, payload.Index)
		})
	}
}

func newRationaleTestServer(router streams.Router, histories history.Service) *Server {
	return NewServer(&fakeMessageStreamApp{
		router:  router,
		history: histories,
		clients: connectedClientManager{},
	})
}

func streamMessage(msgType, msgID, content string) *proto.StreamMessage {
	return &proto.StreamMessage{
		Type:       msgType,
		MsgId:      msgID,
		Content:    content,
		ContactUrn: "ext:217138695938@",
	}
}

func rationaleMessage(msgID, content, index string) *proto.StreamMessage {
	msg := streamMessage("rationale", msgID, content)
	msg.Metadata = map[string]string{"rationale_index": index}
	return msg
}

type publishedStream struct {
	to      string
	payload []byte
}

type recordingRouter struct {
	published []publishedStream
}

func (r *recordingRouter) Start(context.Context) {}

func (r *recordingRouter) Stop(context.Context) {}

func (r *recordingRouter) PublishToClient(_ context.Context, to string, payload []byte) error {
	r.published = append(r.published, publishedStream{
		to:      to,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

type recordingHistory struct {
	saves []history.MessagePayload
}

func (h *recordingHistory) Get(string, string, *time.Time, int, int) ([]history.MessagePayload, error) {
	return nil, nil
}

func (h *recordingHistory) Save(msg history.MessagePayload) error {
	h.saves = append(h.saves, msg)
	return nil
}

type fakeMessageStreamApp struct {
	router  streams.Router
	history history.Service
	clients websocket.ClientManager
}

func (f *fakeMessageStreamApp) Router() streams.Router { return f.router }

func (f *fakeMessageStreamApp) Histories() history.Service { return f.history }

func (f *fakeMessageStreamApp) ClientManager() websocket.ClientManager { return f.clients }

type connectedClientManager struct {
	offline bool
}

func (connectedClientManager) GetConnectedClients() ([]string, error) { return nil, nil }

func (m connectedClientManager) GetConnectedClient(id string) (*websocket.ConnectedClient, error) {
	if m.offline {
		return nil, nil
	}
	return &websocket.ConnectedClient{ID: id}, nil
}

func (connectedClientManager) AddConnectedClient(websocket.ConnectedClient) error { return nil }

func (connectedClientManager) RemoveConnectedClient(string) error { return nil }

func (connectedClientManager) RemoveConnectedClientIf(string, string) (bool, error) {
	return false, nil
}

func (connectedClientManager) UpdateClientTTL(string, int) (bool, error) { return true, nil }

func (connectedClientManager) DefaultClientTTL() int { return 60 }
