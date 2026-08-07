package websocket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveMetricOrigin(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		referer string
		want    string
	}{
		{
			name:    "real origin passes through unchanged",
			origin:  "https://app.example.com",
			referer: "",
			want:    "https://app.example.com",
		},
		{
			name:    "real origin ignores referer",
			origin:  "https://app.example.com",
			referer: "https://something.else.com/page",
			want:    "https://app.example.com",
		},
		{
			name:    "empty origin with no referer falls back to <none>",
			origin:  "",
			referer: "",
			want:    originLabelNone,
		},
		{
			name:    "empty origin recovers scheme+host from referer",
			origin:  "",
			referer: "https://app.example.com/chat/page?token=secret#frag",
			want:    "https://app.example.com",
		},
		{
			name:    "null origin with no referer falls back to <opaque>",
			origin:  "null",
			referer: "",
			want:    originLabelOpaque,
		},
		{
			name:    "null origin recovers real origin from referer",
			origin:  "null",
			referer: "https://parent.example.com/embed",
			want:    "https://parent.example.com",
		},
		{
			name:    "null referer is treated as missing and does not become label",
			origin:  "",
			referer: "null",
			want:    originLabelNone,
		},
		{
			name:    "referer without scheme is rejected",
			origin:  "",
			referer: "app.example.com/page",
			want:    originLabelNone,
		},
		{
			name:    "referer without host is rejected",
			origin:  "",
			referer: "https:///only-path",
			want:    originLabelNone,
		},
		{
			name:    "referer with port is preserved",
			origin:  "",
			referer: "http://localhost:3000/test",
			want:    "http://localhost:3000",
		},
		{
			name:    "null origin with garbage referer still falls back to <opaque>",
			origin:  "null",
			referer: "not a url",
			want:    originLabelOpaque,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveMetricOrigin(tc.origin, tc.referer)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestOriginFromReferer(t *testing.T) {
	tests := []struct {
		name    string
		referer string
		want    string
	}{
		{"empty referer", "", ""},
		{"null referer", "null", ""},
		{"garbage referer", "not a url", ""},
		{"path-only referer", "/local/path", ""},
		{"https with path stripped", "https://app.example.com/chat/page?x=1#frag", "https://app.example.com"},
		{"http with port preserved", "http://localhost:9080/x", "http://localhost:9080"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, originFromReferer(tc.referer))
		})
	}
}

func TestHealthStatusJSON(t *testing.T) {
	status := HealthStatus{
		Redis:                 "ok",
		MongoDB:               "ok",
		RedisLatencySeconds:   0.012,
		MongoDBLatencySeconds: 0.034,
		TotalLatencySeconds:   0.051,
	}

	body, err := json.Marshal(status)
	assert.NoError(t, err)

	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "ok", decoded["redis"])
	assert.Equal(t, "ok", decoded["mongo_db"])
	assert.InDelta(t, 0.012, decoded["redis_latency_seconds"], 0.0001)
	assert.InDelta(t, 0.034, decoded["mongo_db_latency_seconds"], 0.0001)
	assert.InDelta(t, 0.051, decoded["total_latency_seconds"], 0.0001)
}

func TestHealthStatusJSONIncludesLatenciesOnFailure(t *testing.T) {
	status := HealthStatus{
		Redis:                 "connection refused",
		MongoDB:               "ok",
		RedisLatencySeconds:   5.0,
		MongoDBLatencySeconds: 0.02,
		TotalLatencySeconds:   5.02,
	}

	body, err := json.Marshal(status)
	assert.NoError(t, err)

	var decoded map[string]any
	assert.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "connection refused", decoded["redis"])
	assert.InDelta(t, 5.0, decoded["redis_latency_seconds"], 0.0001)
	assert.InDelta(t, 0.02, decoded["mongo_db_latency_seconds"], 0.0001)
	assert.InDelta(t, 5.02, decoded["total_latency_seconds"], 0.0001)
}
