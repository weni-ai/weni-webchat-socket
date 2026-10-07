package websocket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeliverToLocalClientPreservesMessageKind(t *testing.T) {
	pool := NewPool()
	client, ws, server := newTestClient(t)
	defer server.Close()
	defer ws.Close()
	defer client.Conn.Close()

	client.ID = "kind-client"
	pool.Register(client)

	raw := []byte(`{"type":"message","to":"kind-client","from":"agent","message":{"type":"text","text":"thinking"},"message_kind":"rationale"}`)
	err := deliverToLocalClient(pool, stubClientManager{}, client.ID, raw)
	assert.NoError(t, err)

	var received IncomingPayload
	assert.NoError(t, ws.ReadJSON(&received))
	assert.Equal(t, "message", received.Type)
	assert.Equal(t, "kind-client", received.To)
	assert.Equal(t, MessageKindRationale, received.MessageKind)

	encoded, err := json.Marshal(received)
	assert.NoError(t, err)
	assert.Contains(t, string(encoded), `"message_kind":"rationale"`)
}

func TestTryUnmarshalStreamPayloadRationale(t *testing.T) {
	raw := []byte(`{"type":"stream_rationale","id":"msg-1","content":"Vou consultar o status do pedido.","index":1}`)

	payload, ok := tryUnmarshalStreamPayload(raw)
	assert.True(t, ok)

	rationale, ok := payload.(StreamRationalePayload)
	assert.True(t, ok)
	assert.Equal(t, "stream_rationale", rationale.Type)
	assert.Equal(t, "msg-1", rationale.ID)
	assert.Equal(t, "Vou consultar o status do pedido.", rationale.Content)
	assert.Equal(t, 1, rationale.Index)
}

func TestTryUnmarshalStreamEndContainingRationaleText(t *testing.T) {
	raw := []byte(`{"type":"stream_end","id":"msg-1","content":"prefix \"stream_rationale\" suffix"}`)

	payload, ok := tryUnmarshalStreamPayload(raw)
	assert.True(t, ok)

	end, ok := payload.(StreamEndPayload)
	assert.True(t, ok)
	assert.Equal(t, "stream_end", end.Type)
	assert.Equal(t, "msg-1", end.ID)
	assert.Contains(t, end.Content, "stream_rationale")
}
