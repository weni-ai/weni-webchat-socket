package websocket

import (
	"testing"
)

func TestRegister(t *testing.T) {
	pool := NewPool()
	client := &Client{
		ID:       "123",
		Callback: "https://foo.bar",
		Conn:     nil,
	}

	pool.Register(client)

	if len(pool.Clients) != 1 {
		t.Errorf("pool size equal %d, want %d", len(pool.Clients), 1)
	}

	got, found := pool.Clients[client.ID]
	if !found {
		t.Errorf("client was not registered")
	}

	if got != client {
		t.Errorf("want %v, got %v", client, got)
	}
}

func TestUnregister(t *testing.T) {
	client := &Client{
		ID:       "123",
		Callback: "https://foo.bar",
		Conn:     nil,
	}
	pool := Pool{
		Clients: map[string]*Client{
			client.ID: client,
		},
	}

	pool.Unregister(client)

	if len(pool.Clients) != 0 {
		t.Errorf("pool size equal %d, want %d", len(pool.Clients), 0)
	}
}
