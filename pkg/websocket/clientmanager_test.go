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

func TestRemoveConnectedClientIf(t *testing.T) {
	rdbOptions, err := redis.ParseURL("redis://" + envOr("REDIS_HOST", "localhost") + ":6379/1")
	assert.NoError(t, err)
	rdb := redis.NewClient(rdbOptions)
	_ = rdb.FlushDB(context.Background()).Err()
	cm := NewClientManager(rdb, 4)

	assert.NoError(t, cm.AddConnectedClient(ConnectedClient{
		ID:     "cad-1",
		ConnID: "conn-a",
		PodID:  "pod-1",
	}))

	deleted, err := cm.RemoveConnectedClientIf("cad-1", "conn-b")
	assert.NoError(t, err)
	assert.False(t, deleted)
	cc, err := cm.GetConnectedClient("cad-1")
	assert.NoError(t, err)
	assert.NotNil(t, cc)
	assert.Equal(t, "conn-a", cc.ConnID)

	deleted, err = cm.RemoveConnectedClientIf("cad-1", "conn-a")
	assert.NoError(t, err)
	assert.True(t, deleted)
	cc, err = cm.GetConnectedClient("cad-1")
	assert.NoError(t, err)
	assert.Nil(t, cc)
}
