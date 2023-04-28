package websocket

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
)

type ConnectedClient struct {
	ID        string
	AuthToken string
	Channel   string
}

type ClientManager interface {
	GetConnectedClients() ([]string, error)
	GetConnectedClient(string) (*ConnectedClient, error)
	AddConnectedClient(ConnectedClient) error
	RemoveConnectedClient(ConnectedClient) error
	CheckIsConnected(ConnectedClient) (bool, error)
}

type clientManager struct {
	rdb *redis.Client
}

func NewClientManager(redis *redis.Client) ClientManager {
	return &clientManager{rdb: redis}
}

func (m *clientManager) GetConnectedClient(clientID string) (*ConnectedClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	result, err := m.rdb.Get(ctx, clientID).Result()
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

func (m *clientManager) GetConnectedClients() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	connectedClientsCmd := m.rdb.SMembers(ctx, "connected_clients")
	connectedClientsResult, err := connectedClientsCmd.Result()
	if err != nil {
		return nil, err
	}
	log.Println(connectedClientsResult)
	return connectedClientsResult, nil
}

func (m *clientManager) AddConnectedClient(client ConnectedClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_, err := m.rdb.SAdd(ctx, "connected_clients", client.ID).Result()
	if err != nil {
		return err
	}
	_, err = m.rdb.Set(ctx, client.ID, client, 0).Result()
	if err != nil {
		return err
	}
	return nil
}

func (m *clientManager) RemoveConnectedClient(client ConnectedClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	_, err := m.rdb.SRem(ctx, "connected_clients", client.ID).Result()
	if err != nil {
		return err
	}
	_, err = m.rdb.Del(ctx, client.ID).Result()
	if err != nil {
		return err
	}

	return nil
}

func (m *clientManager) CheckIsConnected(client ConnectedClient) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	connectedClients, err := m.rdb.SMembersMap(ctx, "connected_clients").Result()
	if err != nil {
		return false, err
	}
	_, found := connectedClients[client.ID]
	return found, nil
}
