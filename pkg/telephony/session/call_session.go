package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/websocket"
)

// State represents the lifecycle state of a telephony CallSession.
type State string

const (
	StateConnecting State = "connecting"
	StateQueued     State = "queued"
	StateListening  State = "listening"
	StateProcessing State = "processing"
	StateSpeaking   State = "speaking"
	StateError      State = "error"
	StateEnded      State = "ended"
)

var validTransitions = map[State]map[State]struct{}{
	StateConnecting: {StateListening: {}, StateQueued: {}, StateError: {}, StateEnded: {}},
	StateQueued:     {StateConnecting: {}, StateEnded: {}},
	StateListening:  {StateProcessing: {}, StateSpeaking: {}, StateError: {}, StateEnded: {}},
	StateProcessing: {StateSpeaking: {}, StateListening: {}, StateError: {}, StateEnded: {}},
	StateSpeaking:   {StateListening: {}, StateProcessing: {}, StateError: {}, StateEnded: {}},
	StateError:      {StateEnded: {}},
	StateEnded:      {},
}

// CallSession holds per-call state for a single telephony session.
type CallSession struct {
	ID          string
	DID         string
	CallerID    string
	Origin      string
	ChannelUUID string
	ProjectUUID string
	CallbackURL string
	ContactURN  string
	Language    string

	State   State
	StateMu sync.RWMutex

	Conn audiosocket.AudioSocketConn

	VoiceConfig *VoiceConfig
	STT         stt.STTSession

	CurrentTurn *Turn

	// Media loop state (Phase 4+).
	mediaMu                sync.Mutex
	mediaStarted           bool
	audioCh                chan []byte
	mediaDone              chan struct{}
	partialText            string
	lastHandedOffCommitSeq int
	lastHandedOffText      string
	sttFactory             STTSessionFactory
	onCommittedTranscript  CommittedTranscriptHandler

	deliveryMu         sync.Mutex
	deliveryRegistered bool
	grpcMu             sync.Mutex
	lastStreamSeq      int64
	ttsBatcher         *ttsBatcherStub

	CreatedAt time.Time
}

// RegistrationKey returns ContactURN with any scheme: prefix stripped, mirroring
// pkg/grpc/server.go normalizeContactURN. Empty when ContactURN is not yet resolved.
func (cs *CallSession) RegistrationKey() string {
	if cs.ContactURN == "" {
		return ""
	}
	if idx := strings.Index(cs.ContactURN, ":"); idx != -1 {
		return cs.ContactURN[idx+1:]
	}
	return cs.ContactURN
}

// CurrentState returns the session state under read lock.
func (cs *CallSession) CurrentState() State {
	cs.StateMu.RLock()
	defer cs.StateMu.RUnlock()
	return cs.State
}

// transition moves the session to the target state, rejecting invalid transitions.
func (cs *CallSession) transition(to State) error {
	cs.StateMu.Lock()
	defer cs.StateMu.Unlock()

	if cs.State == StateEnded {
		return fmt.Errorf("session %s: invalid transition from %s to %s", cs.ID, cs.State, to)
	}

	allowed, ok := validTransitions[cs.State]
	if !ok {
		return fmt.Errorf("session %s: unknown current state %s", cs.ID, cs.State)
	}
	if _, ok := allowed[to]; !ok {
		return fmt.Errorf("session %s: invalid transition from %s to %s", cs.ID, cs.State, to)
	}

	cs.State = to
	return nil
}

// ttsBatcherStub accumulates delta text until Phase 6 wires the real TTSBatcher.
type ttsBatcherStub struct {
	mu             sync.Mutex
	appendCalls    []string
	lastFlushFinal bool
}

func newTTSBatcherStub() *ttsBatcherStub {
	return &ttsBatcherStub{}
}

func (b *ttsBatcherStub) Append(delta string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.appendCalls = append(b.appendCalls, delta)
}

func (b *ttsBatcherStub) Flush(final bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastFlushFinal = final
}

func (b *ttsBatcherStub) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.appendCalls = nil
	b.lastFlushFinal = false
}

func (b *ttsBatcherStub) AppendCalls() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.appendCalls...)
}

func (b *ttsBatcherStub) LastFlushFinal() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastFlushFinal
}

// handleGRPCPayload unmarshals gRPC stream payloads and dispatches to turn/TTS batching stubs.
func (cs *CallSession) handleGRPCPayload(raw []byte) {
	cs.grpcMu.Lock()
	defer cs.grpcMu.Unlock()

	if cs.ttsBatcher == nil {
		cs.ttsBatcher = newTTSBatcherStub()
	}

	if bytes.Contains(raw, []byte(`"stream_start"`)) {
		var p websocket.StreamStartPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_start" {
			cs.CurrentTurn = &Turn{MsgID: p.ID}
			cs.ttsBatcher.Reset()
			cs.lastStreamSeq = 0
		}
		return
	}

	if bytes.Contains(raw, []byte(`"stream_end"`)) {
		var p websocket.StreamEndPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_end" {
			cs.ttsBatcher.Flush(true)
		}
		return
	}

	if bytes.HasPrefix(raw, []byte(`{"v":`)) {
		var p websocket.StreamDeltaPayload
		if json.Unmarshal(raw, &p) == nil {
			if p.Seq > 0 && p.Seq <= cs.lastStreamSeq {
				return
			}
			if p.Seq > 0 {
				cs.lastStreamSeq = p.Seq
			}
			cs.ttsBatcher.Append(p.V)
		}
	}
}
