package websocket

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

var poolMutex = &sync.Mutex{}

// Pool register all clients
type Pool struct {
	Clients map[string]*Client
}

// NewPool create a pool
func NewPool() *Pool {
	return &Pool{
		Clients: make(map[string]*Client),
	}
}

// Register receive a client and append it in Clients map
func (p *Pool) Register(client *Client) {
	poolMutex.Lock()
	log.Infof("register client %s, pool size: %d", client.ID, len(p.Clients))
	p.Clients[client.ID] = client
	poolMutex.Unlock()
}

// Unregister receive a client instance and if client is in Clients map remove and return pointer to removed client or nil
func (p *Pool) Unregister(client *Client) *Client {
	poolMutex.Lock()
	c := p.Clients[client.ID]
	if p.Clients[client.ID] != nil {
		log.Infof("unregister client %s, pool size: %d", client.ID, len(p.Clients))
		delete(p.Clients, client.ID)
	}
	poolMutex.Unlock()
	return c
}
