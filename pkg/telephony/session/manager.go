package session

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	log "github.com/sirupsen/logrus"
)

// SessionManager tracks in-process CallSession instances.
type SessionManager struct {
	mu            sync.RWMutex
	byID          map[string]*CallSession
	byRegKey      map[string]*CallSession
	flowsClient   flows.IClient
	maxConcurrent int64
	holdAudioPath string
	metrics       *SessionMetrics
}

// NewSessionManager creates a SessionManager with the given dependencies.
func NewSessionManager(
	flowsClient flows.IClient,
	maxConcurrent int64,
	holdAudioPath string,
	metrics *SessionMetrics,
) *SessionManager {
	return &SessionManager{
		byID:          make(map[string]*CallSession),
		byRegKey:      make(map[string]*CallSession),
		flowsClient:   flowsClient,
		maxConcurrent: maxConcurrent,
		holdAudioPath: holdAudioPath,
		metrics:       metrics,
	}
}

// Register resolves the DID, creates a CallSession, and returns its ID.
// When at capacity the session is marked Queued but an ID is still returned.
func (m *SessionManager) Register(did, callerID, origin string) (string, error) {
	channelUUID, projectUUID, callbackURL, err := m.flowsClient.ResolvePSTNChannel(did)
	if err != nil {
		return "", err
	}
	if channelUUID == "" {
		return "", audiosocket.ErrChannelNotFound
	}

	sessionID := uuid.NewString()

	state := StateConnecting
	if int64(m.activeCount()+1) > m.maxConcurrent {
		state = StateQueued
	}

	cs := &CallSession{
		ID:          sessionID,
		DID:         did,
		CallerID:    callerID,
		Origin:      origin,
		ChannelUUID: channelUUID,
		ProjectUUID: projectUUID,
		CallbackURL: callbackURL,
		State:       state,
		CreatedAt:   time.Now(),
	}

	m.mu.Lock()
	if _, exists := m.byID[sessionID]; exists {
		m.mu.Unlock()
		return "", fmt.Errorf("session id collision: %s", sessionID)
	}
	m.byID[sessionID] = cs
	m.mu.Unlock()

	m.refreshGauges()
	return sessionID, nil
}

// Attach binds an AudioSocket connection to a registered session.
func (m *SessionManager) Attach(sessionID string, conn audiosocket.AudioSocketConn) error {
	cs, ok := m.Get(sessionID)
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	cs.Conn = conn

	if cs.CurrentState() == StateQueued {
		m.startHoldAudio(cs)
	}

	return nil
}

// Get returns a session by ID.
func (m *SessionManager) Get(sessionID string) (*CallSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs, ok := m.byID[sessionID]
	return cs, ok
}

// GetByRegistrationKey returns a session by bare contact registration key.
func (m *SessionManager) GetByRegistrationKey(key string) (*CallSession, bool) {
	if key == "" {
		return nil, false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	cs, ok := m.byRegKey[key]
	return cs, ok
}

// SetContactURN records the resolved contact URN and indexes by registration key.
func (m *SessionManager) SetContactURN(sessionID, contactURN string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cs, ok := m.byID[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	cs.ContactURN = contactURN
	if key := cs.RegistrationKey(); key != "" {
		m.byRegKey[key] = cs
	}
	return nil
}

// Remove deletes a session and promotes the earliest queued session if a slot freed.
func (m *SessionManager) Remove(sessionID string) {
	var promoted *CallSession

	m.mu.Lock()
	cs, ok := m.byID[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}

	if key := cs.RegistrationKey(); key != "" {
		delete(m.byRegKey, key)
	}
	delete(m.byID, sessionID)

	hadSlot := cs.State != StateQueued && cs.State != StateEnded
	if hadSlot {
		promoted = m.earliestQueuedLocked()
		if promoted != nil {
			promoted.State = StateConnecting
		}
	}
	m.mu.Unlock()

	if promoted != nil && promoted.Conn != nil && promoted.CurrentState() == StateConnecting {
		// Phase 3 will invoke setup(); queued attach already has the connection.
		log.WithField("session_id", promoted.ID).Info("promoted queued session to connecting")
	}

	m.refreshGauges()
}

func (m *SessionManager) activeCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeCountLocked()
}

func (m *SessionManager) activeCountLocked() int {
	count := 0
	for _, cs := range m.byID {
		if cs.State != StateQueued && cs.State != StateEnded {
			count++
		}
	}
	return count
}

func (m *SessionManager) queuedCountLocked() int {
	count := 0
	for _, cs := range m.byID {
		if cs.State == StateQueued {
			count++
		}
	}
	return count
}

func (m *SessionManager) earliestQueuedLocked() *CallSession {
	var earliest *CallSession
	for _, cs := range m.byID {
		if cs.State != StateQueued {
			continue
		}
		if earliest == nil || cs.CreatedAt.Before(earliest.CreatedAt) {
			earliest = cs
		}
	}
	return earliest
}

func (m *SessionManager) refreshGauges() {
	if m.metrics == nil {
		return
	}
	m.mu.RLock()
	active := m.activeCountLocked()
	queued := m.queuedCountLocked()
	m.mu.RUnlock()
	m.metrics.SetActiveCalls(float64(active))
	m.metrics.SetQueuedCalls(float64(queued))
}

func (m *SessionManager) startHoldAudio(cs *CallSession) {
	if m.holdAudioPath == "" {
		log.WithField("session_id", cs.ID).Warn("hold audio path not configured")
		return
	}

	go func() {
		data, err := os.ReadFile(m.holdAudioPath)
		if err != nil {
			log.WithFields(log.Fields{
				"session_id": cs.ID,
				"path":       m.holdAudioPath,
			}).WithError(err).Error("failed to read hold audio")
			return
		}

		const frameSize = 320
		for cs.CurrentState() == StateQueued {
			for offset := 0; offset < len(data) && cs.CurrentState() == StateQueued; offset += frameSize {
				end := offset + frameSize
				if end > len(data) {
					end = len(data)
				}
				if cs.Conn == nil {
					return
				}
				if err := cs.Conn.WriteAudio(data[offset:end]); err != nil {
					log.WithFields(log.Fields{
						"session_id": cs.ID,
					}).WithError(err).Debug("hold audio write stopped")
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()
}
