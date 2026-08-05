package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockAudioConn struct {
	mu      sync.Mutex
	written [][]byte
	closed  bool
	frames  []audiosocket.Frame
	frameIdx int
}

func (m *mockAudioConn) ReadFrame() (audiosocket.Frame, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frameIdx < len(m.frames) {
		frame := m.frames[m.frameIdx]
		m.frameIdx++
		return frame, nil
	}
	return audiosocket.Frame{}, io.EOF
}

func (m *mockAudioConn) WriteAudio(audio []byte) error {
	m.mu.Lock()
	m.written = append(m.written, append([]byte(nil), audio...))
	m.mu.Unlock()
	return nil
}

func (m *mockAudioConn) WrittenLen() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.written)
}

func (m *mockAudioConn) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockAudioConn) Closed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

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

func newHoldAudioFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hold.pcm")
	require.NoError(t, os.WriteFile(path, make([]byte, 640), 0o644))
	return path
}

func newTestFlowsMock(ctrl *gomock.Controller) *flows.MockIClient {
	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().ResolvePSTNChannel(gomock.Any()).DoAndReturn(
		func(did string) (string, string, string, error) {
			return "ch-" + did, "proj-1", "https://callback", nil
		},
	).AnyTimes()
	mockFlows.EXPECT().GetElevenLabsAPIKey(gomock.Any()).Return("test-key", nil).AnyTimes()
	mockFlows.EXPECT().GetChannelProjectLanguage(gomock.Any()).Return("en", nil).AnyTimes()
	return mockFlows
}

type setupCallTracker struct {
	mu    sync.Mutex
	order []string
}

func (t *setupCallTracker) record(sessionID string) {
	t.mu.Lock()
	t.order = append(t.order, sessionID)
	t.mu.Unlock()
}

func (t *setupCallTracker) Order() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.order...)
}

func TestSessionManagerConcurrentIsolation(t *testing.T) {
	const n = 8
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := newTestFlowsMock(ctrl)
	manager := NewSessionManager(mockFlows, int64(n+2), "", nil, nil)

	type sessionArtifacts struct {
		id        string
		ttsCalls  []string
		turnText  string
		language  string
	}
	artifacts := make([]sessionArtifacts, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			did := fmt.Sprintf("+1555000%04d", idx)
			callerID := fmt.Sprintf("+1556000%04d", idx)
			sessionID, err := manager.Register(did, callerID, "pstn")
			require.NoError(t, err)

			cs, ok := manager.Get(sessionID)
			require.True(t, ok)
			artifacts[idx].id = sessionID

			ttsClient := &isolatedTTSClient{owner: sessionID}
			conn := &mockAudioConn{}
			cs.Conn = conn
			cs.VoiceConfig = &VoiceConfig{
				VoiceID:          fmt.Sprintf("voice-%d", idx),
				Language:         fmt.Sprintf("lang-%d", idx),
				TTSMinBatchChars: 40,
			}
			cs.Language = cs.VoiceConfig.Language
			artifacts[idx].language = cs.Language
			require.NoError(t, cs.transition(StateListening))
			cs.ttsFactory = func(_ *VoiceConfig) tts.TTSStreamClient {
				return ttsClient
			}

			transcript := fmt.Sprintf("transcript-%d", idx)
			cs.handleCommittedTranscript(transcript)
			require.NotNil(t, cs.CurrentTurn)
			artifacts[idx].turnText = cs.CurrentTurn.CommittedText

			cs.handleGRPCPayload([]byte(fmt.Sprintf(`{"type":"stream_start","id":"msg-%d"}`, idx)))
			cs.handleGRPCPayload([]byte(`{"v":"Agent reply. ","seq":1}`))
			cs.handleGRPCPayload([]byte(`{"type":"stream_end","id":"msg"}`))

			deadline := time.After(2 * time.Second)
			for cs.CurrentState() != StateListening {
				select {
				case <-deadline:
					t.Errorf("session %s timed out in state %s", sessionID, cs.CurrentState())
					return
				case <-time.After(10 * time.Millisecond):
				}
			}

			artifacts[idx].ttsCalls = ttsClient.Calls()
			manager.Remove(sessionID)
		}(i)
	}

	wg.Wait()

	seenIDs := make(map[string]struct{}, n)
	for i, art := range artifacts {
		require.NotEmpty(t, art.id)
		seenIDs[art.id] = struct{}{}
		assert.Equal(t, fmt.Sprintf("transcript-%d", i), art.turnText)
		assert.Equal(t, fmt.Sprintf("lang-%d", i), art.language)
		for j := range artifacts {
			if i == j {
				continue
			}
			for _, call := range art.ttsCalls {
				assert.NotContains(t, call, fmt.Sprintf("transcript-%d", j))
			}
		}
	}
	assert.Len(t, seenIDs, n)
}

type isolatedTTSClient struct {
	owner string
	mu    sync.Mutex
	calls []string
}

func (c *isolatedTTSClient) Synthesize(_ context.Context, text, _, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	c.calls = append(c.calls, text)
	c.mu.Unlock()
	ch := make(chan []byte, 1)
	ch <- []byte{0, 0x01}
	close(ch)
	return ch, nil
}

func (c *isolatedTTSClient) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func TestSessionManagerConcurrentRegisterAttachRemoveRace(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race stress test in short mode")
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := newTestFlowsMock(ctrl)
	sttFactory := func(context.Context, *VoiceConfig) (stt.STTSession, error) {
		return &mockSTTSession{}, nil
	}
	ttsFactory := func(*VoiceConfig) tts.TTSStreamClient {
		return &mockTTSClient{audio: make([]byte, 320)}
	}
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, nil, nil, nil)
	manager := NewSessionManager(mockFlows, 20, "", nil, runner)

	const workers = 32
	const opsPerWorker = 20
	var wg sync.WaitGroup
	errCh := make(chan error, workers*opsPerWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				did := fmt.Sprintf("+1555%07d", worker*1000+i)
				sessionID, err := manager.Register(did, "+15559876543", "pstn")
				if err != nil {
					errCh <- err
					continue
				}
				conn := &mockAudioConn{}
				if err := manager.Attach(sessionID, conn); err != nil {
					errCh <- err
				}
				if i%3 == 0 {
					manager.Remove(sessionID)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestSessionManagerCapacityQueueingFIFO(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := newTestFlowsMock(ctrl)
	holdPath := newHoldAudioFile(t)
	tracker := &setupCallTracker{}
	sttFactory := func(_ context.Context, _ *VoiceConfig) (stt.STTSession, error) {
		return &mockSTTSession{}, nil
	}
	ttsFactory := func(_ *VoiceConfig) tts.TTSStreamClient {
		return &mockTTSClient{audio: make([]byte, 320)}
	}
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, nil, nil, nil)
	manager := NewSessionManager(mockFlows, 2, holdPath, nil, runner)

	id1, err := manager.Register("+15551111111", "+15559876543", "pstn")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	id2, err := manager.Register("+15552222222", "+15559876543", "pstn")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	id3, err := manager.Register("+15553333333", "+15559876543", "pstn")
	require.NoError(t, err)
	time.Sleep(5 * time.Millisecond)
	id4, err := manager.Register("+15554444444", "+15559876543", "pstn")
	require.NoError(t, err)

	cs1, _ := manager.Get(id1)
	cs2, _ := manager.Get(id2)
	cs3, _ := manager.Get(id3)
	cs4, _ := manager.Get(id4)

	assert.Equal(t, StateConnecting, cs1.CurrentState())
	assert.Equal(t, StateConnecting, cs2.CurrentState())
	assert.Equal(t, StateQueued, cs3.CurrentState())
	assert.Equal(t, StateQueued, cs4.CurrentState())

	conn3 := &mockAudioConn{}
	conn4 := &mockAudioConn{}
	require.NoError(t, manager.Attach(id3, conn3))
	require.NoError(t, manager.Attach(id4, conn4))

	deadline := time.After(2 * time.Second)
	for conn3.WrittenLen() == 0 || conn4.WrittenLen() == 0 {
		select {
		case <-deadline:
			t.Fatalf("hold audio did not start: conn3=%d conn4=%d frames", conn3.WrittenLen(), conn4.WrittenLen())
		case <-time.After(20 * time.Millisecond):
		}
	}

	manager.Remove(id1)

	deadline = time.After(2 * time.Second)
	for cs3.CurrentState() == StateQueued {
		select {
		case <-deadline:
			t.Fatalf("first queued session not promoted, state=%s", cs3.CurrentState())
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, StateConnecting, cs3.CurrentState())
	tracker.record(id3)
	assert.Equal(t, StateQueued, cs4.CurrentState())

	manager.Remove(id2)

	deadline = time.After(2 * time.Second)
	for cs4.CurrentState() == StateQueued {
		select {
		case <-deadline:
			t.Fatalf("second queued session not promoted, state=%s", cs4.CurrentState())
		case <-time.After(20 * time.Millisecond):
		}
	}
	assert.Equal(t, StateConnecting, cs4.CurrentState())
	tracker.record(id4)
	assert.Equal(t, []string{id3, id4}, tracker.Order())
}

func TestSessionManagerRemoveExecutesFullTeardown(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := newTestFlowsMock(ctrl)
	metrics, err := NewSessionMetrics(nil)
	require.NoError(t, err)

	clientManager := newRecordingClientManager()
	manager := NewSessionManager(mockFlows, 10, "", metrics, nil)
	delivery := NewDeliveryCoordinator(clientManager, manager, "pod-test")
	teardown := &TeardownCoordinator{
		SessionManager:      manager,
		DeliveryCoordinator: delivery,
		Metrics:             metrics,
	}
	manager.SetTeardownCoordinator(teardown)

	sttSession := &mockSTTSession{}
	conn := &mockAudioConn{}

	sessionID, err := manager.Register("+15551111111", "+15559876543", "pstn")
	require.NoError(t, err)

	cs, ok := manager.Get(sessionID)
	require.True(t, ok)
	cs.Conn = conn
	cs.STT = sttSession
	cs.ContactURN = "tel:+15559876543"
	require.NoError(t, RegisterDelivery(cs, clientManager, "pod-test"))
	require.NoError(t, cs.transition(StateListening))

	manager.Remove(sessionID)

	assert.Equal(t, StateEnded, cs.CurrentState())
	assert.True(t, sttSession.closed)
	assert.True(t, conn.Closed())
	assert.Contains(t, clientManager.removed, "+15559876543")
	_, stillRegistered := manager.Get(sessionID)
	assert.False(t, stillRegistered)
	assert.Equal(t, 1.0, metrics.TeardownCount("server_shutdown"))
}
