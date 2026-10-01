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
