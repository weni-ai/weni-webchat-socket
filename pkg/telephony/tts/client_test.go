package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDialer struct {
	serverURL string
}

func (d testDialer) DialContext(ctx context.Context, _ string, requestHeader http.Header) (WebSocketConn, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, d.serverURL, requestHeader)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func newMockTTSServer(t *testing.T) *httptest.Server {
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("xi-api-key"))
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg map[string]interface{}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if text, ok := msg["text"].(string); ok && strings.TrimSpace(text) != "" {
				audio := base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 3})
				_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"audio":"`+audio+`","isFinal":true}`))
				return
			}
		}
	}))
}

func TestClientSynthesizeSuccess(t *testing.T) {
	srv := newMockTTSServer(t)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, "test-api-key", "eleven_flash_v2_5", testDialer{serverURL: wsURL})

	out, err := client.Synthesize(context.Background(), "hello", "voice-1", "en")
	require.NoError(t, err)

	var chunks [][]byte
	for chunk := range out {
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	assert.Equal(t, []byte{0, 1, 2, 3}, chunks[0])
}

func TestClientSynthesizeValidation(t *testing.T) {
	client := NewClient("", "key", "model", nil)

	_, err := client.Synthesize(context.Background(), "hello", "voice-1", "en")
	require.Error(t, err)

	client = NewClient("", "", "model", nil)
	_, err = client.Synthesize(context.Background(), "hello", "voice-1", "en")
	require.Error(t, err)

	client = NewClient("", "key", "model", nil)
	_, err = client.Synthesize(context.Background(), "  ", "voice-1", "en")
	require.Error(t, err)
}

func TestClientSynthesizeDialFailure(t *testing.T) {
	client := NewClient("wss://invalid.example.test", "key", "model", gorillaDialer{})
	_, err := client.Synthesize(context.Background(), "hello", "voice-1", "en")
	require.Error(t, err)
}

func TestBuildTTSURLIncludesLanguage(t *testing.T) {
	urlStr, err := buildTTSURL("wss://api.elevenlabs.io", "voice-1", "eleven_flash_v2_5", "pt")
	require.NoError(t, err)
	assert.Contains(t, urlStr, "language_code=pt")
	assert.Contains(t, urlStr, "output_format=pcm_8000")
}

func TestNormalizeWSBaseURL(t *testing.T) {
	assert.Equal(t, defaultWSBaseURL, normalizeWSBaseURL(""))
	assert.Equal(t, "wss://api.elevenlabs.io", normalizeWSBaseURL("https://api.elevenlabs.io"))
	assert.Equal(t, "ws://localhost", normalizeWSBaseURL("http://localhost"))
}

func TestClientStreamRespectsContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		<-time.After(2 * time.Second)
		_ = conn.Close()
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, "test-api-key", "model", testDialer{serverURL: wsURL})

	ctx, cancel := context.WithCancel(context.Background())
	out, err := client.Synthesize(ctx, "hello", "voice-1", "en")
	require.NoError(t, err)

	cancel()
	deadline := time.After(2 * time.Second)
	for range out {
		select {
		case <-deadline:
			return
		default:
		}
	}
}
