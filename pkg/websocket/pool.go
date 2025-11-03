package websocket

import (
	"sync"

	log "github.com/sirupsen/logrus"
)

var poolMutex = &sync.Mutex{}

// ClientPool register all clients
type ClientPool struct {
	Clients map[string]*Client
}

// NewPool create a pool
func NewPool() *ClientPool {
	return &ClientPool{
		Clients: make(map[string]*Client),
	}
}

// Register receive a client and append it in Clients map
func (p *ClientPool) Register(client *Client) {
	poolMutex.Lock()
	log.Infof("register client %s, pool size: %d", client.ID, len(p.Clients))
	p.Clients[client.ID] = client
	poolMutex.Unlock()
}

// Unregister receive a client instance and if client is in Clients map remove and return pointer to removed client or nil
func (p *ClientPool) Unregister(client *Client) *Client {
	poolMutex.Lock()
	c := p.Clients[client.ID]
	if p.Clients[client.ID] != nil {
		log.Infof("unregister client %s, pool size: %d", client.ID, len(p.Clients))
		delete(p.Clients, client.ID)
	}
	poolMutex.Unlock()
	return c
}

// Find receive clientID and return client from pool with correspondent key if found
func (p *ClientPool) Find(clientID string) (*Client, bool) {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	client, found := p.Clients[clientID]
	return client, found
}

// GetClients returns clients
func (p *ClientPool) GetClients() map[string]*Client {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	return p.Clients
}

// Length return current pool clients length
func (p *ClientPool) Length() int {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	return len(p.Clients)
}

func (p *ClientPool) GetClientsKeys() []string {
	poolMutex.Lock()
	defer poolMutex.Unlock()
	kClients := make([]string, 0, len(p.Clients))
	for k := range p.Clients {
		kClients = append(kClients, k)
	}
	return kClients
}
