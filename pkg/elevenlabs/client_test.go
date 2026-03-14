package elevenlabs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	c := NewClient("my-key", "https://api.elevenlabs.io/")

	assert.Equal(t, "my-key", c.apiKey)
	assert.Equal(t, "https://api.elevenlabs.io", c.apiURL, "trailing slash should be trimmed")
	assert.NotNil(t, c.httpClient)
}

func TestNewClient_NoTrailingSlash(t *testing.T) {
	c := NewClient("key", "https://api.elevenlabs.io")
	assert.Equal(t, "https://api.elevenlabs.io", c.apiURL)
}

func TestFetchToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "test-api-key", r.Header.Get("xi-api-key"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.True(t, strings.HasSuffix(r.URL.Path, "/v1/single-use-token/realtime_scribe"))

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: "stt-token-123"})
	}))
	defer server.Close()

	c := NewClient("test-api-key", server.URL)
	token, err := c.fetchToken("realtime_scribe")

	require.NoError(t, err)
	assert.Equal(t, "stt-token-123", token)
}

func TestFetchToken_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer server.Close()

	c := NewClient("bad-key", server.URL)
	token, err := c.fetchToken("realtime_scribe")

	assert.Empty(t, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "token request failed with status 401")
}

func TestFetchToken_EmptyToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: ""})
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	token, err := c.fetchToken("tts_websocket")

	assert.Empty(t, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty token")
}

func TestFetchToken_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	token, err := c.fetchToken("realtime_scribe")

	assert.Empty(t, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestFetchToken_NetworkError(t *testing.T) {
	c := NewClient("key", "http://localhost:1")
	token, err := c.fetchToken("realtime_scribe")

	assert.Empty(t, token)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request realtime_scribe token")
}

func TestRequestSingleUseTokens_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var token string
		if strings.Contains(r.URL.Path, "realtime_scribe") {
			token = "stt-abc"
		} else if strings.Contains(r.URL.Path, "tts_websocket") {
			token = "tts-xyz"
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: token})
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	stt, tts, err := c.RequestSingleUseTokens()

	require.NoError(t, err)
	assert.Equal(t, "stt-abc", stt)
	assert.Equal(t, "tts-xyz", tts)
}

func TestRequestSingleUseTokens_FetchesBothInParallel(t *testing.T) {
	var requestCount int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		var token string
		if strings.Contains(r.URL.Path, "realtime_scribe") {
			token = "stt"
		} else {
			token = "tts"
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: token})
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	stt, tts, err := c.RequestSingleUseTokens()

	require.NoError(t, err)
	assert.Equal(t, "stt", stt)
	assert.Equal(t, "tts", tts)
	assert.Equal(t, int64(2), atomic.LoadInt64(&requestCount))
}

func TestRequestSingleUseTokens_STTError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "realtime_scribe") {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("server error"))
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: "tts-ok"})
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	stt, tts, err := c.RequestSingleUseTokens()

	assert.Empty(t, stt)
	assert.Empty(t, tts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "realtime_scribe")
}

func TestRequestSingleUseTokens_TTSError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "tts_websocket") {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("forbidden"))
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(tokenResponse{Token: "stt-ok"})
	}))
	defer server.Close()

	c := NewClient("key", server.URL)
	stt, tts, err := c.RequestSingleUseTokens()

	assert.Empty(t, stt)
	assert.Empty(t, tts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "tts_websocket")
}

func TestRequestSingleUseTokens_BothFail(t *testing.T) {
	c := NewClient("key", "http://localhost:1")
	stt, tts, err := c.RequestSingleUseTokens()

	assert.Empty(t, stt)
	assert.Empty(t, tts)
	assert.Error(t, err)
}

func TestClientImplementsIClient(t *testing.T) {
	var _ IClient = (*Client)(nil)
}
