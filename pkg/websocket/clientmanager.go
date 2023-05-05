package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	// ClientConnectionKeyPrefix is the prefix of every key in redis for client connection, key example "client:foo_123"
	ClientConnectionKeyPrefix = "client:"
	// ClientTTL is the TTL expiration for client connection
	ClientTTL = 4
)

// ConnectedClient represents the struct of a client connected with the main infos
type ConnectedClient struct {
	ID        string `json:"id,omitempty"`
	AuthToken string `json:"auth_token,omitempty"`
	Channel   string `json:"channel,omitempty"`
}

func (c ConnectedClient) MarshalBinary() ([]byte, error) {
	return json.Marshal(c)
}

func (c *ConnectedClient) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, c)
}

type ClientManager interface {
	GetConnectedClients() ([]string, error)
	GetConnectedClient(string) (*ConnectedClient, error)
	AddConnectedClient(ConnectedClient) error
	RemoveConnectedClient(string) error
	UpdateTTL(string, int) (bool, error)
}

type clientManager struct {
	rdb *redis.Client
}

// NewClientManager return a instance of a client manager that uses redis client for persistence
func NewClientManager(redis *redis.Client) ClientManager {
	return &clientManager{rdb: redis}
}

// GetConnectedClient returns the ConnectedClient searched by its clientID
func (m *clientManager) GetConnectedClient(clientID string) (*ConnectedClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	result, err := m.rdb.Get(ctx, ClientConnectionKeyPrefix+clientID).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	connectedClient := &ConnectedClient{}
	err = json.Unmarshal([]byte(result), connectedClient)
	if err != nil {
		return nil, err
	}
	return connectedClient, nil
}

// GetConnectedClients returns a slice of connected clients keys
func (m *clientManager) GetConnectedClients() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	ccs, _, err := m.rdb.Scan(ctx, 0, ClientConnectionKeyPrefix+"*", 0).Result()
	if err != nil {
		return nil, err
	}
	return ccs, nil
}

// AddConnectedClient add a client connection from ConnectedClient with a ttl defined by clientTTL constant
func (m *clientManager) AddConnectedClient(client ConnectedClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_, err := m.rdb.Set(ctx, ClientConnectionKeyPrefix+client.ID, client, time.Second*ClientTTL).Result()
	if err != nil {
		return err
	}
	return nil
}

// RemoveConnectedClient removes the connected client by its clientID
func (m *clientManager) RemoveConnectedClient(clientID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_, err := m.rdb.Del(ctx, ClientConnectionKeyPrefix+clientID).Result()
	if err != nil {
		return err
	}
	return nil
}

// UpdateTTL updates key expiration
func (m *clientManager) UpdateTTL(clientID string, expiration int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return m.rdb.Expire(ctx, clientID, time.Second*time.Duration(expiration)).Result()
}
