package memcache

import (
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	cacheDomains := New[string, []string]()
	chanUUID := "5f610454-98c1-4a54-9499-e2d2b9b68334"
	cacheDomains.Set(chanUUID, []string{"127.0.0.1", "localhost"}, time.Duration(time.Second*2))

	chan1domains, ok := cacheDomains.Get(chanUUID)
	if !ok {
		t.Error("Expected channel UUID to be found")
	}
	if len(chan1domains) != 2 {
		t.Error("Expected 2 domains in cache, got", len(chan1domains))
	}
	time.Sleep(3 * time.Second)

	_, ok = cacheDomains.Get(chanUUID)
	if ok {
		t.Error("Expected channel UUID not to be found")
	}
}
