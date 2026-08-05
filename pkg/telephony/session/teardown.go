package session

import (
	"time"

	log "github.com/sirupsen/logrus"
)

// TeardownCoordinator wires session teardown dependencies shared across CallSessions.
type TeardownCoordinator struct {
	SessionManager      *SessionManager
	DeliveryCoordinator *DeliveryCoordinator
	Metrics             *SessionMetrics
	onRemove            func(sessionID string)
}

// Bind attaches teardown dependencies to a CallSession.
func (tc *TeardownCoordinator) Bind(cs *CallSession) {
	if tc == nil || cs == nil {
		return
	}
	cs.teardown = tc
}

// Complete tears down a session and releases its manager slot.
func (tc *TeardownCoordinator) Complete(cs *CallSession, reason string) {
	if cs == nil {
		return
	}
	hadSlot := cs.CurrentState() != StateQueued && cs.CurrentState() != StateEnded
	cs.Teardown(reason)
	if tc != nil && tc.SessionManager != nil {
		if _, still := tc.SessionManager.Get(cs.ID); still {
			tc.SessionManager.removeSession(cs.ID, hadSlot)
		}
		return
	}
	if tc != nil && tc.onRemove != nil {
		tc.onRemove(cs.ID)
	}
}

// Teardown performs idempotent call resource cleanup for any termination reason.
func (cs *CallSession) Teardown(reason string) {
	cs.teardownOnce.Do(func() {
		cs.doTeardown(reason)
	})
}

// TeardownReason returns the reason recorded for the last teardown (for tests).
func (cs *CallSession) TeardownReason() string {
	cs.teardownMu.Lock()
	defer cs.teardownMu.Unlock()
	return cs.teardownReason
}

func (cs *CallSession) doTeardown(reason string) {
	cs.teardownMu.Lock()
	cs.teardownReason = reason
	cs.teardownMu.Unlock()

	cs.stopMediaLoop()
	cs.stopTTSStream()

	if cs.STT != nil {
		_ = cs.STT.Close()
		cs.STT = nil
	}

	if cs.teardown != nil && cs.teardown.DeliveryCoordinator != nil {
		cs.teardown.DeliveryCoordinator.TeardownDelivery(cs)
	}

	if cs.Conn != nil {
		_ = cs.Conn.Close()
		cs.Conn = nil
	}

	cs.forceTransition(StateEnded)

	metrics := cs.metrics
	if cs.teardown != nil && cs.teardown.Metrics != nil {
		metrics = cs.teardown.Metrics
	}
	if metrics != nil {
		metrics.IncCallTeardown(reason)
	}

	log.WithFields(cs.logFields()).WithField("reason", reason).Info("telephony session torn down")
}

func (cs *CallSession) stopMediaLoop() {
	cs.mediaMu.Lock()
	done := cs.mediaDone
	started := cs.mediaStarted
	cs.mediaMu.Unlock()

	if !started {
		return
	}

	if cs.Conn != nil {
		_ = cs.Conn.Close()
	}

	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			log.WithFields(cs.logFields()).Warn("telephony: timed out waiting for media loop shutdown")
		}
	}
}

func (cs *CallSession) forceTransition(to State) {
	cs.StateMu.Lock()
	defer cs.StateMu.Unlock()

	if cs.State == to {
		return
	}

	cs.State = to
	cs.ensureBargeIn()
	cs.BargeIn.SetArmed(to == StateSpeaking)
}
