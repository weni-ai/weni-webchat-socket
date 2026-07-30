package stt

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultWSBaseURL     = "wss://api.elevenlabs.io"
	realtimePath         = "/v1/speech-to-text/realtime"
	sessionReadyTimeout  = 10 * time.Second
	defaultEventsBufSize = 32
)

// EventKind identifies an STT session event type.
type EventKind int

const (
	EventPartialTranscript EventKind = iota
	EventCommittedTranscript
	EventClosed
)

// Event is a tagged union of STT session events.
type Event struct {
	Kind EventKind

	PartialTranscript   PartialTranscript
	CommittedTranscript CommittedTranscript
	Closed              Closed
}

// PartialTranscript carries in-progress recognition text.
type PartialTranscript struct {
	Text string
}

// CommittedTranscript carries a finalized recognition result.
type CommittedTranscript struct {
	Text string
}

// Closed signals the STT session ended.
type Closed struct {
	Err error
}

// STTSession abstracts a gateway-side ElevenLabs STT WebSocket session.
type STTSession interface {
	Send(audio []byte) error
	Events() <-chan Event
	Close() error
}

// SessionConfig holds parameters for opening an ElevenLabs STT session.
type SessionConfig struct {
	APIKey       string
	ModelID      string
	Language     string
	VADSilenceMs int64
}

// WebSocketDialer dials an ElevenLabs STT WebSocket.
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

// Client opens gateway-side ElevenLabs Scribe v2 Realtime sessions.
type Client struct {
	wsBaseURL string
	dialer    WebSocketDialer
}

// NewClient creates an STT client. wsBaseURL accepts http(s) or wss URLs.
func NewClient(wsBaseURL string, dialer WebSocketDialer) *Client {
	if dialer == nil {
		dialer = gorillaDialer{}
	}
	return &Client{
		wsBaseURL: normalizeWSBaseURL(wsBaseURL),
		dialer:    dialer,
	}
}

// OpenSession dials ElevenLabs and waits for session_started before returning.
func (c *Client) OpenSession(ctx context.Context, cfg SessionConfig) (STTSession, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("stt: missing ElevenLabs API key")
	}

	wsURL, err := buildSTTURL(c.wsBaseURL, cfg)
	if err != nil {
		return nil, err
	}

	headers := http.Header{}
	headers.Set("xi-api-key", cfg.APIKey)

	conn, err := c.dialer.DialContext(ctx, wsURL, headers)
	if err != nil {
		return nil, fmt.Errorf("stt: dial websocket: %w", err)
	}

	s := &session{
		conn:   conn,
		events: make(chan Event, defaultEventsBufSize),
		done:   make(chan struct{}),
	}

	ready := make(chan error, 1)
	go s.readLoop(ready)

	select {
	case err := <-ready:
		if err != nil {
			_ = s.Close()
			return nil, err
		}
		return s, nil
	case <-ctx.Done():
		_ = s.Close()
		return nil, ctx.Err()
	case <-time.After(sessionReadyTimeout):
		_ = s.Close()
		return nil, fmt.Errorf("stt: timed out waiting for session_started")
	}
}

type session struct {
	conn   WebSocketConn
	events chan Event
	done   chan struct{}
	closeOnce sync.Once
}

func (s *session) Send(audio []byte) error {
	payload, err := json.Marshal(map[string]interface{}{
		"message_type":  "input_audio_chunk",
		"audio_base_64": base64.StdEncoding.EncodeToString(audio),
		"commit":        false,
		"sample_rate":   16000,
	})
	if err != nil {
		return err
	}
	return s.conn.WriteMessage(websocket.TextMessage, payload)
}

func (s *session) Events() <-chan Event {
	return s.events
}

func (s *session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		err = s.conn.Close()
		select {
		case s.events <- Event{Kind: EventClosed, Closed: Closed{Err: nil}}:
		default:
		}
		close(s.events)
	})
	return err
}

func (s *session) readLoop(ready chan<- error) {
	defer func() {
		_ = s.Close()
	}()

	for {
		_, data, err := s.conn.ReadMessage()
		if err != nil {
			select {
			case s.events <- Event{Kind: EventClosed, Closed: Closed{Err: err}}:
			default:
			}
			return
		}

		var envelope struct {
			MessageType string          `json:"message_type"`
			Text        string          `json:"text"`
			Error       string          `json:"error"`
			Raw         json.RawMessage `json:"-"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}

		switch envelope.MessageType {
		case "session_started":
			select {
			case ready <- nil:
			default:
			}
		case "partial_transcript":
			s.events <- Event{
				Kind:              EventPartialTranscript,
				PartialTranscript: PartialTranscript{Text: envelope.Text},
			}
		case "committed_transcript":
			s.events <- Event{
				Kind:                EventCommittedTranscript,
				CommittedTranscript: CommittedTranscript{Text: envelope.Text},
			}
		case "auth_error", "error", "quota_exceeded", "transcriber_error":
			sttErr := fmt.Errorf("stt: %s", envelope.MessageType)
			if envelope.Error != "" {
				sttErr = fmt.Errorf("stt: %s: %s", envelope.MessageType, envelope.Error)
			}
			select {
			case ready <- sttErr:
			default:
				s.events <- Event{Kind: EventClosed, Closed: Closed{Err: sttErr}}
			}
			return
		}
	}
}

func buildSTTURL(wsBaseURL string, cfg SessionConfig) (string, error) {
	u, err := url.Parse(wsBaseURL + realtimePath)
	if err != nil {
		return "", err
	}

	q := u.Query()
	if cfg.ModelID != "" {
		q.Set("model_id", cfg.ModelID)
	}
	q.Set("audio_format", "pcm_16000")
	q.Set("commit_strategy", "vad")
	if cfg.Language != "" {
		q.Set("language_code", cfg.Language)
	}
	if cfg.VADSilenceMs > 0 {
		secs := float64(cfg.VADSilenceMs) / 1000.0
		q.Set("vad_silence_threshold_secs", strconv.FormatFloat(secs, 'f', -1, 64))
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
