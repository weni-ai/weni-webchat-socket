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
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
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

	ttsFactory    TTSClientFactory
	metrics       *SessionMetrics
	ttsBatcher    *tts.TTSBatcher
	ttsWriterMu   sync.Mutex
	ttsWriterDone chan struct{}
	ttsPlaybackMu sync.Mutex
	lastGaplessPlayback bool
	lastBatchMarkers    []int

	BargeIn *BargeInController

	bargeInMu            sync.Mutex
	lastBargeInLatency   time.Duration

	holdAudioMu      sync.Mutex
	holdAudioRunning bool

	teardownOnce   sync.Once
	teardownMu     sync.Mutex
	teardownReason string
	teardown       *TeardownCoordinator

	lastAudioAt time.Time

	CreatedAt time.Time
}

// UpdateLanguage applies a mid-call language change. The trigger mechanism is owned by
// Flows/platform and out of this repo's scope; this method is the consumption point only.
// The updated language is used on the next STT reconnect and subsequent TTS batcher instances.
func (cs *CallSession) UpdateLanguage(lang string) {
	lang = NormalizeLanguageCode(lang)
	cs.StateMu.Lock()
	defer cs.StateMu.Unlock()
	cs.Language = lang
	if cs.VoiceConfig != nil {
		cs.VoiceConfig.Language = lang
	}
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
	cs.ensureBargeIn()
	cs.BargeIn.SetArmed(to == StateSpeaking)
	return nil
}

func (cs *CallSession) ensureBargeIn() {
	if cs.BargeIn != nil {
		return
	}
	cs.BargeIn = NewBargeInController(cs.handleBargeIn)
}

// LastBargeInLatency returns the latency recorded for the most recent barge-in (for tests).
func (cs *CallSession) LastBargeInLatency() time.Duration {
	cs.bargeInMu.Lock()
	defer cs.bargeInMu.Unlock()
	return cs.lastBargeInLatency
}

func (cs *CallSession) recordBargeInLatency(latency time.Duration) {
	cs.bargeInMu.Lock()
	cs.lastBargeInLatency = latency
	cs.bargeInMu.Unlock()
}

// handleGRPCPayload unmarshals gRPC stream payloads and dispatches to TTS batching/playback.
func (cs *CallSession) handleGRPCPayload(raw []byte) {
	if bytes.Contains(raw, []byte(`"stream_start"`)) {
		var p websocket.StreamStartPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_start" {
			cs.grpcMu.Lock()
			cs.lastStreamSeq = 0
			cs.grpcMu.Unlock()
			cs.startTTSStream(p.ID)
		}
		return
	}

	if bytes.Contains(raw, []byte(`"stream_end"`)) {
		var p websocket.StreamEndPayload
		if json.Unmarshal(raw, &p) == nil && p.Type == "stream_end" {
			cs.flushTTSStream()
		}
		return
	}

	if bytes.HasPrefix(raw, []byte(`{"v":`)) {
		var p websocket.StreamDeltaPayload
		if json.Unmarshal(raw, &p) == nil {
			cs.grpcMu.Lock()
			if p.Seq > 0 && p.Seq <= cs.lastStreamSeq {
				cs.grpcMu.Unlock()
				return
			}
			if p.Seq > 0 {
				cs.lastStreamSeq = p.Seq
			}
			cs.grpcMu.Unlock()
			cs.appendTTSDelta(p.V)
		}
	}
}
