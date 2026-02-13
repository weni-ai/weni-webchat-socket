package websocket

import (
	"context"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
)

func TestClientManager(t *testing.T) {
	rdbOptions, err := redis.ParseURL("redis://" + envOr("REDIS_HOST", "localhost") + ":6379/1")
	assert.NoError(t, err)
	rdb := redis.NewClient(rdbOptions)
	cm := NewClientManager(rdb, 4)

	newClientID := "foo_id_123"
	newClient := ConnectedClient{ID: newClientID}

	client, err := cm.GetConnectedClient(newClient.ID)
	assert.NoError(t, err)
	assert.Nil(t, client)

	err = cm.AddConnectedClient(newClient)
	assert.NoError(t, err)

	connClients, err := cm.GetConnectedClients()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(connClients))

	client, err = cm.GetConnectedClient(newClient.ID)
	assert.NoError(t, err)
	assert.NotNil(t, client)

	err = cm.RemoveConnectedClient(newClient.ID)
	assert.NoError(t, err)

	err = cm.RemoveConnectedClient(newClient.ID)
	assert.NoError(t, err)

	client, err = cm.GetConnectedClient(newClient.ID)
	assert.NoError(t, err)
	assert.Nil(t, client)

	err = cm.AddConnectedClient(newClient)
	assert.NoError(t, err)

	// TTL-based expiry is no longer used; explicit removal is required
	err = cm.RemoveConnectedClient(newClient.ID)
	assert.NoError(t, err)

	client, err = cm.GetConnectedClient(newClient.ID)
	assert.NoError(t, err)
	assert.Nil(t, client)

	rdb.Del(context.TODO(), "connected_clients")
}
