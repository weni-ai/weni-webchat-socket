package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

const (
	// ClientConnectionKeyPrefix is the prefix of every key in redis for client connection, key example "client:foo_123"
	ClientConnectionKeyPrefix = "client:"
)

// ConnectedClient represents the struct of a client connected with the main infos
type ConnectedClient struct {
	ID        string `json:"id,omitempty"`
	AuthToken string `json:"auth_token,omitempty"`
	Channel   string `json:"channel,omitempty"`
	PodID     string `json:"pod_id,omitempty"`
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
	UpdateClientTTL(string, int) (bool, error)
	DefaultClientTTL() int
}

type clientManager struct {
	rdb       *redis.Client
	clientTTL int
}

// NewClientManager return a instance of a client manager that uses redis client for persistence
func NewClientManager(redis *redis.Client, clientTTL int) ClientManager {
	return &clientManager{rdb: redis, clientTTL: clientTTL}
}

// GetConnectedClient returns the ConnectedClient searched by its clientID
func (m *clientManager) GetConnectedClient(clientID string) (*ConnectedClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	ccs, _, err := m.rdb.Scan(ctx, 0, ClientConnectionKeyPrefix+"*", 0).Result()
	if err != nil {
		return nil, err
	}
	return ccs, nil
}

// AddConnectedClient add a client connection from ConnectedClient with a ttl defined by clientTTL constant
func (m *clientManager) AddConnectedClient(client ConnectedClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	_, err := m.rdb.Set(ctx, ClientConnectionKeyPrefix+client.ID, client, time.Second*time.Duration(m.clientTTL)).Result()
	if err != nil {
		return err
	}
	return nil
}

// RemoveConnectedClient removes the connected client by its clientID
func (m *clientManager) RemoveConnectedClient(clientID string) error {
	log.Debugf("removing connected client %s", clientID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	_, err := m.rdb.Del(ctx, ClientConnectionKeyPrefix+clientID).Result()
	if err != nil {
		return err
	}
	return nil
}

// UpdateClientTTL updates key expiration
func (m *clientManager) UpdateClientTTL(clientID string, expiration int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	key := ClientConnectionKeyPrefix + clientID
	return m.rdb.Expire(ctx, key, time.Second*time.Duration(expiration)).Result()
}

func (m *clientManager) DefaultClientTTL() int { return m.clientTTL }
