package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistrationKeyStripsTelScheme(t *testing.T) {
	cs := &CallSession{ContactURN: "tel:+15559876543"}
	assert.Equal(t, "+15559876543", cs.RegistrationKey())
}

func TestRegistrationKeyEmptyWhenUnresolved(t *testing.T) {
	cs := &CallSession{}
	assert.Equal(t, "", cs.RegistrationKey())
}

func TestRegistrationKeyBareIdentifierUnchanged(t *testing.T) {
	cs := &CallSession{ContactURN: "+15559876543"}
	assert.Equal(t, "+15559876543", cs.RegistrationKey())
}

func TestTransitionRejectsEndedToAnything(t *testing.T) {
	cs := &CallSession{ID: "sess-1", State: StateEnded}
	err := cs.transition(StateListening)
	assert.Error(t, err)
	assert.Equal(t, StateEnded, cs.State)
}

func TestTransitionConnectingToListening(t *testing.T) {
	cs := &CallSession{ID: "sess-1", State: StateConnecting}
	err := cs.transition(StateListening)
	assert.NoError(t, err)
	assert.Equal(t, StateListening, cs.State)
}

func TestTransitionInvalidPath(t *testing.T) {
	cs := &CallSession{ID: "sess-1", State: StateConnecting}
	err := cs.transition(StateSpeaking)
	assert.Error(t, err)
	assert.Equal(t, StateConnecting, cs.State)
}
