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

func (p *Pool) Register(client *Client) {
	poolMutex.Lock()
	log.Infof("register client %s, pool size: %d", client.ID, len(p.Clients))
	p.Clients[client.ID] = client
	poolMutex.Unlock()
}

func (p *Pool) Unregister(client *Client) {
	poolMutex.Lock()
	log.Infof("unregister client %s, pool size: %d", client.ID, len(p.Clients))
	delete(p.Clients, client.ID)
	poolMutex.Unlock()
}
