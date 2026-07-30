package stt

import (
	"context"
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

func newMockSTTServer(t *testing.T, onConnect func(*websocket.Conn)) *httptest.Server {
	upgrader := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("xi-api-key"))
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		onConnect(conn)
	}))
}

func TestClientOpenSessionSuccess(t *testing.T) {
	srv := newMockSTTServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"session_started","session_id":"s1"}`))
		<-time.After(100 * time.Millisecond)
		_ = conn.Close()
	})
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, testDialer{serverURL: wsURL})

	session, err := client.OpenSession(context.Background(), SessionConfig{
		APIKey:       "test-api-key",
		ModelID:      "scribe_v2_realtime",
		Language:     "en",
		VADSilenceMs: 1500,
	})
	require.NoError(t, err)
	require.NotNil(t, session)
	require.NoError(t, session.Close())
}

func TestClientOpenSessionAuthFailure(t *testing.T) {
	srv := newMockSTTServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"auth_error","error":"invalid api key"}`))
	})
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, testDialer{serverURL: wsURL})

	_, err := client.OpenSession(context.Background(), SessionConfig{
		APIKey:  "test-api-key",
		ModelID: "scribe_v2_realtime",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auth_error")
}

func TestClientOpenSessionUnavailable(t *testing.T) {
	client := NewClient("wss://invalid.example.test", gorillaDialer{})

	_, err := client.OpenSession(context.Background(), SessionConfig{
		APIKey:  "test-api-key",
		ModelID: "scribe_v2_realtime",
	})
	require.Error(t, err)
}

func TestClientOpenSessionMissingAPIKey(t *testing.T) {
	client := NewClient("wss://api.elevenlabs.io", nil)
	_, err := client.OpenSession(context.Background(), SessionConfig{ModelID: "scribe_v2_realtime"})
	require.Error(t, err)
}

func TestBuildSTTURLIncludesVADParams(t *testing.T) {
	urlStr, err := buildSTTURL("wss://api.elevenlabs.io", SessionConfig{
		ModelID:      "scribe_v2_realtime",
		Language:     "en",
		VADSilenceMs: 1500,
	})
	require.NoError(t, err)
	assert.Contains(t, urlStr, "commit_strategy=vad")
	assert.Contains(t, urlStr, "audio_format=pcm_16000")
	assert.Contains(t, urlStr, "language_code=en")
	assert.Contains(t, urlStr, "vad_silence_threshold_secs=1.5")
}

func TestSessionSendInputAudioChunk(t *testing.T) {
	received := make(chan map[string]interface{}, 1)
	srv := newMockSTTServer(t, func(conn *websocket.Conn) {
		require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"session_started"}`)))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		payload := map[string]interface{}{}
		require.NoError(t, json.Unmarshal(msg, &payload))
		received <- payload
	})
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, testDialer{serverURL: wsURL})
	session, err := client.OpenSession(context.Background(), SessionConfig{
		APIKey:  "test-api-key",
		ModelID: "scribe_v2_realtime",
	})
	require.NoError(t, err)

	require.NoError(t, session.Send([]byte{1, 2, 3}))

	select {
	case payload := <-received:
		assert.Equal(t, "input_audio_chunk", payload["message_type"])
		assert.Equal(t, false, payload["commit"])
		assert.Equal(t, float64(16000), payload["sample_rate"])
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for audio chunk")
	}
	require.NoError(t, session.Close())
}

func TestSessionPartialAndCommittedEvents(t *testing.T) {
	srv := newMockSTTServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"session_started"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"partial_transcript","text":"hel"}`))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"message_type":"committed_transcript","text":"hello"}`))
	})
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	client := NewClient(wsURL, testDialer{serverURL: wsURL})
	session, err := client.OpenSession(context.Background(), SessionConfig{
		APIKey:  "test-api-key",
		ModelID: "scribe_v2_realtime",
	})
	require.NoError(t, err)

	var events []Event
	timeout := time.After(2 * time.Second)
readLoop:
	for {
		select {
		case evt, ok := <-session.Events():
			if !ok {
				break readLoop
			}
			events = append(events, evt)
			if len(events) >= 2 {
				break readLoop
			}
		case <-timeout:
			t.Fatal("timed out waiting for STT events")
		}
	}

	require.GreaterOrEqual(t, len(events), 2)
	assert.Equal(t, EventPartialTranscript, events[0].Kind)
	assert.Equal(t, "hel", events[0].PartialTranscript.Text)
	assert.Equal(t, EventCommittedTranscript, events[1].Kind)
	assert.Equal(t, "hello", events[1].CommittedTranscript.Text)
	require.NoError(t, session.Close())
}

func TestNormalizeWSBaseURL(t *testing.T) {
	assert.Equal(t, "wss://api.elevenlabs.io", normalizeWSBaseURL("https://api.elevenlabs.io"))
	assert.Equal(t, "wss://api.elevenlabs.io", normalizeWSBaseURL(""))
}
