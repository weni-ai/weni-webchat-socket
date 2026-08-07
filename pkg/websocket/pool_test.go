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
	pool := ClientPool{
		Clients: map[string]*Client{
			client.ID: client,
		},
	}

	pool.Unregister(client)

	if len(pool.Clients) != 0 {
		t.Errorf("pool size equal %d, want %d", len(pool.Clients), 0)
	}
}

func TestUnregisterIgnoresMismatchedPointer(t *testing.T) {
	owner := &Client{ID: "same-id", Callback: "https://foo.bar"}
	stale := &Client{ID: "same-id", Callback: "https://foo.bar"}
	pool := NewPool()
	pool.Register(owner)

	removed := pool.Unregister(stale)
	if removed != nil {
		t.Fatalf("expected nil for mismatched pointer, got %v", removed)
	}
	got, found := pool.Find("same-id")
	if !found || got != owner {
		t.Fatalf("owner should remain in pool")
	}
}

func TestForceClose(t *testing.T) {
	client := &Client{ID: "fc-1", Callback: "https://foo.bar"}
	pool := NewPool()
	pool.Register(client)

	removed := pool.ForceClose("fc-1")
	if removed != client {
		t.Fatalf("ForceClose should return the client, got %v", removed)
	}
	if _, found := pool.Find("fc-1"); found {
		t.Fatalf("client should be removed from pool")
	}
	if pool.ForceClose("fc-1") != nil {
		t.Fatalf("ForceClose on missing client should return nil")
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
