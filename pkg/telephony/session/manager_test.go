package session

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAudioConn struct {
	written [][]byte
}

func (m *mockAudioConn) ReadFrame() (audiosocket.Frame, error) {
	return audiosocket.Frame{}, nil
}

func (m *mockAudioConn) WriteAudio(audio []byte) error {
	m.written = append(m.written, append([]byte(nil), audio...))
	return nil
}

func (m *mockAudioConn) Close() error { return nil }

func TestSessionManagerRegisterAttachHappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().ResolvePSTNChannel("+15551234567").Return("ch-1", "proj-1", "https://callback", nil)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("test-key", nil)
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("en", nil)

	manager := NewSessionManager(mockFlows, 10, "", nil, nil)

	sessionID, err := manager.Register("+15551234567", "+15559876543", "pstn")
	require.NoError(t, err)
	require.NotEmpty(t, sessionID)

	cs, ok := manager.Get(sessionID)
	require.True(t, ok)
	assert.Equal(t, StateConnecting, cs.CurrentState())
	assert.Equal(t, "ch-1", cs.ChannelUUID)
	assert.Equal(t, "test-key", cs.VoiceConfig.ElevenLabsAPIKey)

	conn := &mockAudioConn{}
	err = manager.Attach(sessionID, conn)
	require.NoError(t, err)
	assert.Equal(t, conn, cs.Conn)
}

func TestSessionManagerRegisterUnknownDID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().ResolvePSTNChannel("+15551234567").Return("", "", "", nil)

	manager := NewSessionManager(mockFlows, 10, "", nil, nil)

	_, err := manager.Register("+15551234567", "+15559876543", "pstn")
	require.Error(t, err)
	assert.ErrorIs(t, err, audiosocket.ErrChannelNotFound)
}

func TestSessionManagerRegisterSTTDependencyDown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().ResolvePSTNChannel("+15551234567").Return("ch-1", "proj-1", "https://callback", nil)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("", nil)
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("en", nil)

	manager := NewSessionManager(mockFlows, 10, "", nil, nil)

	_, err := manager.Register("+15551234567", "+15559876543", "pstn")
	require.Error(t, err)
	assert.ErrorIs(t, err, audiosocket.ErrSTTDependencyDown)
}
