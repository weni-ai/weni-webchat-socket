package websocket

import (
	log "github.com/sirupsen/logrus"
)

// Pool register all clients
type Pool struct {
	Register   chan *Client
	Unregister chan *Client
	Clients    map[string]*Client
	Sender     chan Sender
}

// NewPool create a pool
func NewPool() *Pool {
	return &Pool{
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Clients:    make(map[string]*Client),
		Sender:     make(chan Sender),
	}
}

// Start our pool
func (p *Pool) Start() {
	for {
		select {
		case client := <-p.Register:
			log.Tracef("Registering client: %s", client.ID)
			p.Clients[client.ID] = client
			log.Tracef("Pool size: %d", len(p.Clients))
			client.Conn.WriteJSON(
				ExternalPayload{
					Text: "Welcome to weni webchat",
				},
			)
			break
		case client := <-p.Unregister:
			log.Tracef("Unregistering client: %s", client.ID)
			delete(p.Clients, client.ID)
			log.Tracef("Pool size: %d", len(p.Clients))
			break
		case sender := <-p.Sender:
			log.Tracef("Sending message: %#v to client: %q", sender.Payload, sender.Client.ID)
			if err := sender.Client.Conn.WriteJSON(sender.Payload); err != nil {
				log.Error(err)
				return
			}
		}
	}
}
