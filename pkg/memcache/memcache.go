package memcache

import (
	"sync"
	"time"
)

type Cache[K comparable, V any] struct {
	items map[K]item[V]
	mu    sync.Mutex
}

type item[V any] struct {
	value   V
	expiry  time.Time
	deleted bool
}

func New[K comparable, V any]() *Cache[K, V] {
	return &Cache[K, V]{
		items: make(map[K]item[V]),
	}
}

func (c *Cache[K, V]) Set(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = item[V]{
		value:  value,
		expiry: time.Now().Add(ttl),
	}
}

func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	item, found := c.items[key]
	if !found || time.Now().After(item.expiry) || item.deleted {
		delete(c.items, key)
		return item.value, false
	}
	return item.value, true
}

func (c *Cache[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, key)
}
