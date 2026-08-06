package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

const defaultWSBaseURL = "wss://api.elevenlabs.io"

// TTSStreamClient abstracts a gateway-side ElevenLabs streaming TTS client.
type TTSStreamClient interface {
	Synthesize(ctx context.Context, text, voiceID, language string) (<-chan []byte, error)
}

// WebSocketDialer dials an ElevenLabs TTS WebSocket.
type WebSocketDialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (WebSocketConn, error)
}

// WebSocketConn abstracts a WebSocket connection for testability.
type WebSocketConn interface {
	ReadMessage() (messageType int, p []byte, err error)
	WriteMessage(messageType int, data []byte) error
	Close() error
}

type gorillaDialer struct{}

func (gorillaDialer) DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (WebSocketConn, error) {
	d := websocket.Dialer{}
	conn, _, err := d.DialContext(ctx, urlStr, requestHeader)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// Client synthesizes speech through ElevenLabs streaming TTS.
type Client struct {
	wsBaseURL string
	apiKey    string
	modelID   string
	dialer    WebSocketDialer
}

// NewClient creates a TTS client bound to a tenant API key.
func NewClient(wsBaseURL, apiKey, modelID string, dialer WebSocketDialer) *Client {
	if dialer == nil {
		dialer = gorillaDialer{}
	}
	if modelID == "" {
		modelID = "eleven_flash_v2_5"
	}
	return &Client{
		wsBaseURL: normalizeWSBaseURL(wsBaseURL),
		apiKey:    apiKey,
		modelID:   modelID,
		dialer:    dialer,
	}
}

// Synthesize opens a streaming TTS session for a single text utterance.
func (c *Client) Synthesize(ctx context.Context, text, voiceID, language string) (<-chan []byte, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("tts: missing ElevenLabs API key")
	}
	if voiceID == "" {
		return nil, fmt.Errorf("tts: missing voice ID")
	}
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("tts: empty text")
	}

	wsURL, err := buildTTSURL(c.wsBaseURL, voiceID, c.modelID, language)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("xi-api-key", c.apiKey)

	conn, err := c.dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("tts: dial websocket: %w", err)
	}

	out := make(chan []byte, 8)
	go c.stream(ctx, conn, text, out)
	return out, nil
}

func (c *Client) stream(ctx context.Context, conn WebSocketConn, text string, out chan<- []byte) {
	defer close(out)
	defer conn.Close()

	initMsg := map[string]interface{}{
		"text": " ",
		"voice_settings": map[string]interface{}{
			"stability":        0.5,
			"similarity_boost": 0.8,
		},
	}
	if err := writeJSON(conn, initMsg); err != nil {
		return
	}
	if err := writeJSON(conn, map[string]interface{}{"text": text}); err != nil {
		return
	}
	if err := writeJSON(conn, map[string]interface{}{"text": ""}); err != nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}

		var msg struct {
			Audio   string `json:"audio"`
			IsFinal bool   `json:"isFinal"`
		}
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg.Audio != "" {
			chunk, err := base64.StdEncoding.DecodeString(msg.Audio)
			if err != nil {
				continue
			}
			select {
			case out <- chunk:
			case <-ctx.Done():
				return
			}
		}
		if msg.IsFinal {
			return
		}
	}
}

func writeJSON(conn WebSocketConn, payload map[string]interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func buildTTSURL(wsBaseURL, voiceID, modelID, language string) (string, error) {
	path := fmt.Sprintf("/v1/text-to-speech/%s/stream-input", url.PathEscape(voiceID))
	u, err := url.Parse(wsBaseURL + path)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("model_id", modelID)
	q.Set("output_format", "pcm_8000")
	if language != "" {
		q.Set("language_code", language)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func normalizeWSBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultWSBaseURL
	}
	raw = strings.TrimRight(raw, "/")
	raw = strings.Replace(raw, "https://", "wss://", 1)
	raw = strings.Replace(raw, "http://", "ws://", 1)
	return raw
}
