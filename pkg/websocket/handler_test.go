package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ilhasoft/wwcs/pkg/history"
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

func TestShouldSaveToHistory(t *testing.T) {
	tests := []struct {
		name    string
		payload IncomingPayload
		want    bool
	}{
		{
			name:    "normal message is saved",
			payload: IncomingPayload{Type: "message"},
			want:    true,
		},
		{
			name:    "final response is saved",
			payload: IncomingPayload{Type: "message", MessageKind: MessageKindFinalResponse},
			want:    true,
		},
		{
			name:    "rationale is not saved",
			payload: IncomingPayload{Type: "message", MessageKind: MessageKindRationale},
			want:    false,
		},
		{
			name:    "typing start is not saved",
			payload: IncomingPayload{Type: "typing_start"},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldSaveToHistory(tc.payload))
		})
	}
}

func TestSendHandlerHistoryAndPublish(t *testing.T) {
	const to = "urn:1"
	tests := []struct {
		name       string
		body       string
		expectSave bool
		wantKind   string
	}{
		{
			name:       "normal message is saved and published",
			body:       `{"type":"message","to":"urn:1","from":"agent","channel_uuid":"ch","message":{"type":"text","timestamp":"1616700878","text":"hello"}}`,
			expectSave: true,
		},
		{
			name:       "rationale is published and not saved",
			body:       `{"type":"message","to":"urn:1","from":"agent","channel_uuid":"ch","message":{"type":"text","timestamp":"1616700878","text":"thinking"},"message_kind":"rationale"}`,
			expectSave: false,
			wantKind:   MessageKindRationale,
		},
		{
			name:       "typing start is published and not saved",
			body:       `{"type":"typing_start","to":"urn:1","from":"agent"}`,
			expectSave: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			histories := history.NewMockService(ctrl)
			if tc.expectSave {
				histories.EXPECT().Save(gomock.Any()).Return(nil).Times(1)
			}

			router := &recordingRouter{}
			app := &App{
				Histories: histories,
				ClientManager: stubClientManager{
					client: &ConnectedClient{ID: to, PodID: "pod-1"},
				},
				Router: router,
			}

			req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			app.SendHandler(rec, req)

			assert.Equal(t, http.StatusAccepted, rec.Code)
			assert.Len(t, router.payloads, 1)

			var published IncomingPayload
			assert.NoError(t, json.Unmarshal(router.payloads[0], &published))
			assert.Equal(t, to, router.to[0])
			assert.Equal(t, tc.wantKind, published.MessageKind)
			if tc.wantKind != "" {
				assert.Contains(t, string(router.payloads[0]), `"message_kind":"`+tc.wantKind+`"`)
			} else {
				assert.NotContains(t, string(router.payloads[0]), "message_kind")
			}
		})
	}
}

type recordingRouter struct {
	payloads [][]byte
	to       []string
}

func (r *recordingRouter) Start(context.Context) {}

func (r *recordingRouter) Stop(context.Context) {}

func (r *recordingRouter) PublishToClient(_ context.Context, to string, payload []byte) error {
	r.to = append(r.to, to)
	r.payloads = append(r.payloads, append([]byte(nil), payload...))
	return nil
}

type stubClientManager struct {
	client *ConnectedClient
	err    error
}

func (s stubClientManager) GetConnectedClient(string) (*ConnectedClient, error) {
	return s.client, s.err
}

func (s stubClientManager) GetConnectedClients() ([]string, error) { return nil, nil }

func (s stubClientManager) AddConnectedClient(ConnectedClient) error { return nil }

func (s stubClientManager) RemoveConnectedClient(string) error { return nil }

func (s stubClientManager) RemoveConnectedClientIf(string, string) (bool, error) {
	return false, nil
}

func (s stubClientManager) UpdateClientTTL(string, int) (bool, error) { return true, nil }

func (s stubClientManager) DefaultClientTTL() int { return 60 }
