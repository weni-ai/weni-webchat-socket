package websocket

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

// ClientPool register all clients
type ClientPool struct {
	Clients map[string]*Client
	mu      sync.RWMutex
}

// NewPool create a pool
func NewPool() *ClientPool {
	return &ClientPool{
		Clients: make(map[string]*Client),
	}
}

// Register receive a client and append it in Clients map
func (p *ClientPool) Register(client *Client) {
	p.mu.Lock()
	log.Debugf("register client %s, pool size: %d", client.ID, len(p.Clients))
	p.Clients[client.ID] = client
	p.mu.Unlock()
}

// Unregister receive a client instance and if client is in Clients map remove and return pointer to removed client or nil
func (p *ClientPool) Unregister(client *Client) *Client {
	p.mu.Lock()
	c := p.Clients[client.ID]
	if p.Clients[client.ID] != nil {
		log.Debugf("unregister client %s, pool size: %d", client.ID, len(p.Clients))
		delete(p.Clients, client.ID)
	}
	p.mu.Unlock()
	return c
}

// Find receive clientID and return client from pool with correspondent key if found
func (p *ClientPool) Find(clientID string) (*Client, bool) {
	p.mu.RLock()
	client, found := p.Clients[clientID]
	p.mu.RUnlock()
	return client, found
}

// GetClients returns clients
func (p *ClientPool) GetClients() map[string]*Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.Clients
}

// Length return current pool clients length
func (p *ClientPool) Length() int {
	p.mu.RLock()
	l := len(p.Clients)
	p.mu.RUnlock()
	return l
}

func (p *ClientPool) GetClientsKeys() []string {
	p.mu.RLock()
	kClients := make([]string, 0, len(p.Clients))
	for k := range p.Clients {
		kClients = append(kClients, k)
	}
	p.mu.RUnlock()
	return kClients
}
