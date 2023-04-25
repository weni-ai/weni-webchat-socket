package websocket

import (
	"log"
	"sync"
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

func TestFind(t *testing.T) {
	pool := NewPool()
	client := &Client{
		ID:       "123",
		Callback: "https://foo.bar",
		Conn:     nil,
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		pool.Register(client)

		got, found := pool.Find(client.ID)
		if !found {
			t.Errorf("client was not registered")
		}

		if got != client {
			t.Errorf("want %v, got %v", client, got)
		}
		log.Println("client registered")
		wg.Done()
	}()

	go func() {
		c, _ := pool.Find(client.ID)
		pool.Unregister(c)

		got, found := pool.Find(c.ID)
		if found {
			t.Errorf("client yet registered")
		}

		if got != nil {
			t.Errorf("want %v, got %v", client, got)
		}
		log.Println("client unregistered")
		wg.Done()
	}()
}

func TestLength(t *testing.T) {
	pool := NewPool()
	client := &Client{
		ID:       "123",
		Callback: "https://foo.bar",
		Conn:     nil,
	}

	pool.Register(client)

	if pool.Length() != 1 {
		t.Errorf("pool size equal %d, want %d", pool.Length(), 1)
	}
}
