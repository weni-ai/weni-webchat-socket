package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupAudioKeepaliveSendsSilence(t *testing.T) {
	conn := &mockAudioConn{}
	k := startSetupAudioKeepalive(conn)

	time.Sleep(50 * time.Millisecond)
	k.Stop()

	require.NotEmpty(t, conn.Written())
	for _, frame := range conn.Written() {
		assert.Len(t, frame, audioFrameSize)
		assert.Equal(t, make([]byte, audioFrameSize), frame)
	}
}

func TestSetupAudioKeepalivePauseStopsSilence(t *testing.T) {
	conn := &mockAudioConn{}
	k := startSetupAudioKeepalive(conn)

	time.Sleep(30 * time.Millisecond)
	beforePause := conn.WrittenLen()

	k.Pause()
	time.Sleep(50 * time.Millisecond)
	k.Stop()

	assert.Equal(t, beforePause, conn.WrittenLen())
}

func TestSetupAudioKeepaliveNilConn(t *testing.T) {
	k := startSetupAudioKeepalive(nil)
	k.Stop()
}
