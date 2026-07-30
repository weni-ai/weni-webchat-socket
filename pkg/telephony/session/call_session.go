package session

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
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
