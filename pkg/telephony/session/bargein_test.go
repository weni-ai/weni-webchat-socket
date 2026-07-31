package session

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBargeInControllerArmedGating(t *testing.T) {
	var triggers atomic.Int32
	bic := NewBargeInController(func(time.Time) {
		triggers.Add(1)
	})

	bic.SetArmed(false)
	bic.Trigger()
	assert.Equal(t, int32(0), triggers.Load())

	bic.SetArmed(true)
	bic.Trigger()
	assert.Equal(t, int32(1), triggers.Load())

	bic.SetArmed(false)
	bic.Trigger()
	assert.Equal(t, int32(1), triggers.Load())
}

func TestBargeInControllerArmedOnlyWhileSpeaking(t *testing.T) {
	cs := &CallSession{ID: "sess-armed", State: StateConnecting}
	cs.ensureBargeIn()

	require.NoError(t, cs.transition(StateListening))
	assert.False(t, cs.BargeIn.IsArmed())

	require.NoError(t, cs.transition(StateProcessing))
	assert.False(t, cs.BargeIn.IsArmed())

	require.NoError(t, cs.transition(StateSpeaking))
	assert.True(t, cs.BargeIn.IsArmed())

	require.NoError(t, cs.transition(StateListening))
	assert.False(t, cs.BargeIn.IsArmed())
}
