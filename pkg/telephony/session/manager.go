package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	log "github.com/sirupsen/logrus"
)

// SessionManager tracks in-process CallSession instances.
type SessionManager struct {
	mu                  sync.RWMutex
	byID                map[string]*CallSession
	byRegKey            map[string]*CallSession
	flowsClient         flows.IClient
	maxConcurrent       int64
	holdAudioPath       string
	metrics             *SessionMetrics
	setupRunner         *SetupRunner
	teardownCoordinator *TeardownCoordinator
}

// NewSessionManager creates a SessionManager with the given dependencies.
func NewSessionManager(
	flowsClient flows.IClient,
	maxConcurrent int64,
	holdAudioPath string,
	metrics *SessionMetrics,
	setupRunner *SetupRunner,
) *SessionManager {
	m := &SessionManager{
		byID:          make(map[string]*CallSession),
		byRegKey:      make(map[string]*CallSession),
		flowsClient:   flowsClient,
		maxConcurrent: maxConcurrent,
		holdAudioPath: holdAudioPath,
		metrics:       metrics,
		setupRunner:   setupRunner,
	}
	if setupRunner != nil && setupRunner.onRemove == nil {
		setupRunner.onRemove = func(sessionID string) {
			m.removeSession(sessionID, true)
		}
	}
	return m
}

// SetSetupRunner attaches the setup runner after dependent wiring is complete.
func (m *SessionManager) SetSetupRunner(runner *SetupRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setupRunner = runner
	if runner != nil && runner.onRemove == nil {
		runner.onRemove = func(sessionID string) {
			m.removeSession(sessionID, true)
		}
	}
}

// SetTeardownCoordinator wires teardown dependencies for registered sessions.
func (m *SessionManager) SetTeardownCoordinator(coordinator *TeardownCoordinator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teardownCoordinator = coordinator
	if coordinator != nil {
		coordinator.SessionManager = m
	}
}

// TeardownAll tears down every active session (e.g. process shutdown).
func (m *SessionManager) TeardownAll(reason string) {
	m.mu.RLock()
	sessions := make([]*CallSession, 0, len(m.byID))
	for _, cs := range m.byID {
		if cs.CurrentState() != StateEnded {
			sessions = append(sessions, cs)
		}
	}
	m.mu.RUnlock()

	for _, cs := range sessions {
		m.ensureSessionTeardown(cs)
		if cs.teardown != nil {
			cs.teardown.Complete(cs, reason)
			continue
		}
		hadSlot := cs.CurrentState() != StateQueued && cs.CurrentState() != StateEnded
		cs.Teardown(reason)
		m.removeSession(cs.ID, hadSlot)
	}
}

// Register resolves the DID, creates a CallSession, and returns its ID.
// When at capacity the session is marked Queued but an ID is still returned.
func (m *SessionManager) Register(did, callerID, origin string) (string, error) {
	channelUUID, projectUUID, err := m.flowsClient.ResolvePSTNChannel(did)
	if err != nil {
		return "", err
	}
	if channelUUID == "" {
		return "", audiosocket.ErrChannelNotFound
	}

	voiceConfig, err := ResolveVoiceConfig(m.flowsClient, channelUUID)
	if err != nil {
		return "", err
	}
	if voiceConfig.ElevenLabsAPIKey == "" {
		return "", audiosocket.ErrSTTDependencyDown
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
		VoiceConfig: voiceConfig,
		Language:    voiceConfig.Language,
		State:       state,
		CreatedAt:   time.Now(),
	}

	m.mu.Lock()
	if _, exists := m.byID[sessionID]; exists {
		m.mu.Unlock()
		return "", fmt.Errorf("session id collision: %s", sessionID)
	}
	m.byID[sessionID] = cs
	if m.teardownCoordinator != nil {
		m.teardownCoordinator.Bind(cs)
	}
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
		cs.StartHoldAudioLoop(m.holdAudioPath)
		return nil
	}

	if cs.CurrentState() == StateConnecting && m.setupRunner != nil {
		m.setupRunner.Run(cs)
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
// When the session is still active, Remove executes the same teardown path as caller hangup.
func (m *SessionManager) Remove(sessionID string) {
	cs, ok := m.Get(sessionID)
	if !ok {
		return
	}
	m.ensureSessionTeardown(cs)
	if cs.teardown != nil {
		cs.teardown.Complete(cs, "server_shutdown")
		return
	}
	hadSlot := cs.CurrentState() != StateQueued && cs.CurrentState() != StateEnded
	cs.Teardown("server_shutdown")
	m.removeSession(sessionID, hadSlot)
}

func (m *SessionManager) ensureSessionTeardown(cs *CallSession) {
	if cs.teardown != nil && cs.teardown.SessionManager != nil {
		return
	}
	if m.teardownCoordinator != nil {
		m.teardownCoordinator.Bind(cs)
		return
	}
	if cs.teardown == nil {
		cs.teardown = &TeardownCoordinator{SessionManager: m}
		return
	}
	cs.teardown.SessionManager = m
}

func (m *SessionManager) removeSession(sessionID string, hadSlot bool) {
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

	if hadSlot {
		promoted = m.earliestQueuedLocked()
	}
	m.mu.Unlock()

	if promoted != nil {
		if err := promoted.transition(StateConnecting); err != nil {
			log.WithFields(promoted.logFields()).WithError(err).Warn("failed to promote queued session")
		} else if promoted.Conn != nil {
			log.WithFields(promoted.logFields()).Info("promoted queued session to connecting")
			if m.setupRunner != nil {
				m.setupRunner.Run(promoted)
			}
		}
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
		state := cs.CurrentState()
		if state != StateQueued && state != StateEnded {
			count++
		}
	}
	return count
}

func (m *SessionManager) queuedCountLocked() int {
	count := 0
	for _, cs := range m.byID {
		if cs.CurrentState() == StateQueued {
			count++
		}
	}
	return count
}

func (m *SessionManager) earliestQueuedLocked() *CallSession {
	var earliest *CallSession
	for _, cs := range m.byID {
		if cs.CurrentState() != StateQueued {
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

