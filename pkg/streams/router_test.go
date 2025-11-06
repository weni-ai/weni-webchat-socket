package streams

import (
	"context"
	"encoding/json"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func newRedisForTest(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("WWC_REDIS_QUEUE_URL")
	if url == "" {
		url = "redis://localhost:6379/1"
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Skip("invalid redis url, skipping streams tests: ", err)
	}
	rdb := redis.NewClient(opts)
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skip("redis not available, skipping streams tests: ", err)
	}
	return rdb
}

func TestRouter_PublishAndConsume_Local(t *testing.T) {
	rdb := newRedisForTest(t)
	_ = rdb.FlushDB(context.Background()).Err()
	podID := "test-pod-1"

	var delivered int32
	clientID := "client-123"

	lookup := func(id string) (string, bool, error) {
		if id == clientID {
			return podID, true, nil
		}
		return "", false, nil
	}
	isLocal := func(id string) bool { return id == clientID }
	deliver := func(id string, raw []byte) error {
		// sanity check JSON
		var v map[string]interface{}
		_ = json.Unmarshal(raw, &v)
		atomic.AddInt32(&delivered, 1)
		return nil
	}

	cfg := StreamsConfig{StreamsMaxLenApprox: 1000, StreamsReadCount: 10, StreamsBlockMs: 100, StreamsClaimIdleMs: 1000, HeartbeatTTLSeconds: 5}
	router := NewRouter(rdb, podID, cfg, lookup, isLocal, deliver)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	router.Start(ctx)

	payload := []byte(`{"type":"message","to":"x"}`)
	if err := router.PublishToClient(context.Background(), clientID, payload); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	// wait for delivery
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&delivered) > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected delivery, got %d", delivered)
}
