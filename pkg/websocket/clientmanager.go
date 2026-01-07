package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
)

const (
	// ClientsHashKey stores clientID -> ConnectedClient JSON mappings
	ClientsHashKey = "ws:clients"
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
	// Prefer hash-based mapping
	result, err := m.rdb.HGet(ctx, ClientsHashKey, clientID).Result()
	if err == redis.Nil {
		return nil, nil
	} else if err != nil {
		log.WithFields(log.Fields{
			"client_id": clientID,
			"hash_key":  ClientsHashKey,
			"timeout":   m.clientTTL,
		}).WithError(err).Error("Redis: failed to get connected client from hash")
		return nil, err
	}
	connectedClient := &ConnectedClient{}
	if err := json.Unmarshal([]byte(result), connectedClient); err != nil {
		log.WithFields(log.Fields{
			"client_id": clientID,
			"raw_data":  result,
		}).WithError(err).Error("Redis: failed to unmarshal connected client data")
		return nil, err
	}
	return connectedClient, nil
}

// GetConnectedClients returns a slice of connected clients keys
func (m *clientManager) GetConnectedClients() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	ids, err := m.rdb.HKeys(ctx, ClientsHashKey).Result()
	if err != nil {
		log.WithFields(log.Fields{
			"hash_key": ClientsHashKey,
			"timeout":  m.clientTTL,
		}).WithError(err).Error("Redis: failed to get connected clients list")
		return nil, err
	}
	return ids, nil
}

// AddConnectedClient add a client connection from ConnectedClient with a ttl defined by clientTTL constant
func (m *clientManager) AddConnectedClient(client ConnectedClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	b, err := json.Marshal(client)
	if err != nil {
		log.WithFields(log.Fields{
			"client_id": client.ID,
			"channel":   client.Channel,
			"pod_id":    client.PodID,
		}).WithError(err).Error("Redis: failed to marshal client data for storage")
		return err
	}
	if err := m.rdb.HSet(ctx, ClientsHashKey, client.ID, b).Err(); err != nil {
		log.WithFields(log.Fields{
			"client_id": client.ID,
			"channel":   client.Channel,
			"pod_id":    client.PodID,
			"hash_key":  ClientsHashKey,
		}).WithError(err).Error("Redis: failed to add connected client to hash")
		return err
	}
	return nil
}

// RemoveConnectedClient removes the connected client by its clientID
func (m *clientManager) RemoveConnectedClient(clientID string) error {
	log.Debugf("removing connected client %s", clientID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	if err := m.rdb.HDel(ctx, ClientsHashKey, clientID).Err(); err != nil {
		log.WithFields(log.Fields{
			"client_id": clientID,
			"hash_key":  ClientsHashKey,
		}).WithError(err).Error("Redis: failed to remove connected client from hash")
		return err
	}
	// Also remove last-seen bookkeeping to prevent unbounded ZSET growth
	if err := m.rdb.ZRem(ctx, ClientsHashKey+":lastseen", clientID).Err(); err != nil {
		log.WithFields(log.Fields{
			"client_id": clientID,
			"zset_key":  ClientsHashKey + ":lastseen",
		}).WithError(err).Warn("Redis: failed to remove client from lastseen ZSET")
	}
	return nil
}

// UpdateClientTTL updates key expiration
func (m *clientManager) UpdateClientTTL(clientID string, expiration int) (bool, error) {
	// No TTL on per-field hash entries; record last-seen timestamp for observability
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(m.clientTTL))
	defer cancel()
	zkey := ClientsHashKey + ":lastseen"
	score := float64(time.Now().Unix())
	if err := m.rdb.ZAdd(ctx, zkey, &redis.Z{Score: score, Member: clientID}).Err(); err != nil {
		log.WithFields(log.Fields{
			"client_id": clientID,
			"zset_key":  zkey,
			"score":     score,
		}).WithError(err).Error("Redis: failed to update client last-seen timestamp")
		return false, err
	}
	return true, nil
}

func (m *clientManager) DefaultClientTTL() int { return m.clientTTL }
