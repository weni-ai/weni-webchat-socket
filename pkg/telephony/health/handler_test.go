package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerMethodNotAllowed(t *testing.T) {
	handler := &Handler{
		RDB: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}),
	}

	req := httptest.NewRequest(http.MethodPost, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandlerRedisUnavailable(t *testing.T) {
	handler := &Handler{
		RDB:     redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: time.Millisecond}),
		Timeout: time.Second,
	}

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var status Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	assert.NotEqual(t, "ok", status.Redis)
}

func TestHandlerRedisOK(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379", DialTimeout: time.Second})
	ctx, cancel := contextWithTimeout(t, time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skip("redis not available for health handler test")
	}

	handler := &Handler{
		RDB:     rdb,
		Timeout: time.Second,
	}

	req := httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var status Status
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	assert.Equal(t, "ok", status.Redis)
}

func contextWithTimeout(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), timeout)
}
