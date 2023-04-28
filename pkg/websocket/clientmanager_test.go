package websocket

import (
	"context"
	"testing"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	"github.com/stretchr/testify/assert"
)

func TestClientManager(t *testing.T) {
	rdbOptions, err := redis.ParseURL(config.Get().RedisQueue.URL)
	assert.NoError(t, err)
	rdb := redis.NewClient(rdbOptions)
	cm := NewClientManager(rdb)

	newClientID := "foo_id_123"
	newClient := ConnectedClient{ID: newClientID}
	err = cm.AddConnectedClient(newClient)
	assert.NoError(t, err)

	connClients, err := cm.GetConnectedClients()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(connClients))

	found, err := cm.CheckIsConnected(newClient)
	assert.NoError(t, err)
	assert.True(t, found)

	err = cm.RemoveConnectedClient(newClient)
	assert.NoError(t, err)

	err = cm.RemoveConnectedClient(newClient)
	assert.NoError(t, err)

	found, err = cm.CheckIsConnected(newClient)
	assert.NoError(t, err)
	assert.False(t, found)

	rdb.Del(context.TODO(), "connected_clients")
}
