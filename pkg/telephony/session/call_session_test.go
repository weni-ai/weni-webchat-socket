package session

import (
	"context"
	"io"
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

type mockSTTSession struct {
	closed bool
}

func (m *mockSTTSession) Send([]byte) error { return nil }
func (m *mockSTTSession) Events() <-chan stt.Event {
	ch := make(chan stt.Event)
	close(ch)
	return ch
}
func (m *mockSTTSession) Close() error {
	m.closed = true
	return nil
}

type mockTTSClient struct {
	audio []byte
}

func (m *mockTTSClient) Synthesize(ctx context.Context, text, voiceID, language string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	if len(m.audio) > 0 {
		ch <- append([]byte(nil), m.audio...)
	}
	close(ch)
	return ch, nil
}

func TestSetupRunnerFullSequence(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("test-key", nil).AnyTimes()
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("en", nil).AnyTimes()

	sttSession := &mockSTTSession{}
	sttFactory := func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		return sttSession, nil
	}
	ttsClient := &mockTTSClient{audio: make([]byte, 640)}
	ttsFactory := func(cfg *VoiceConfig) tts.TTSStreamClient {
		return ttsClient
	}

	var removed sync.WaitGroup
	removed.Add(1)
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, nil, nil, func(sessionID string) {
		removed.Done()
	})

	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:          "sess-1",
		ChannelUUID: "ch-1",
		Language:    "en",
		State:       StateConnecting,
		Conn:        conn,
		VoiceConfig: &VoiceConfig{
			ElevenLabsAPIKey: "test-key",
			VoiceID:          "voice-1",
			Language:         "en",
		},
	}

	runner.run(cs)

	assert.Equal(t, StateListening, cs.CurrentState())
	assert.NotNil(t, cs.STT)
	assert.NotEmpty(t, conn.written)
}

func TestSetupRunnerSTTFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	sttFactory := func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		return nil, assert.AnError
	}
	ttsFactory := func(cfg *VoiceConfig) tts.TTSStreamClient {
		return &mockTTSClient{audio: make([]byte, 320)}
	}

	var removed sync.WaitGroup
	removed.Add(1)
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, nil, nil, func(sessionID string) {
		removed.Done()
	})

	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:          "sess-2",
		ChannelUUID: "ch-1",
		Language:    "en",
		State:       StateConnecting,
		Conn:        conn,
		VoiceConfig: &VoiceConfig{
			ElevenLabsAPIKey: "test-key",
			VoiceID:          "voice-1",
			Language:         "en",
		},
	}

	runner.run(cs)

	assert.Equal(t, StateEnded, cs.CurrentState())
	assert.Nil(t, cs.STT)
	assert.NotEmpty(t, conn.written)

	done := make(chan struct{})
	go func() {
		removed.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("teardown did not remove session")
	}
}

func TestSetupRunnerChannelResolutionFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("", assert.AnError)

	sttFactory := func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		t.Fatal("STT should not open when channel resolution fails")
		return nil, nil
	}
	ttsFactory := func(cfg *VoiceConfig) tts.TTSStreamClient {
		return &mockTTSClient{audio: make([]byte, 320)}
	}

	var removed sync.WaitGroup
	removed.Add(1)
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, nil, nil, func(sessionID string) {
		removed.Done()
	})

	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:          "sess-3",
		ChannelUUID: "ch-1",
		Language:    "en",
		State:       StateConnecting,
		Conn:        conn,
	}

	runner.run(cs)
	assert.Equal(t, StateEnded, cs.CurrentState())
}

func TestResolveGreetingText(t *testing.T) {
	text := ResolveGreetingText("en")
	assert.NotEmpty(t, text)
}

func TestResolveSpokenTextFallbackToEnglish(t *testing.T) {
	text := ResolveSpokenText("voice.greeting", "unknown-lang")
	assert.Equal(t, spokenCatalog["voice.greeting"]["en"], text)
}

type eventfulSTTSession struct {
	events chan stt.Event
	closed bool
	sent   [][]byte
}

func newEventfulSTTSession(events ...stt.Event) *eventfulSTTSession {
	ch := make(chan stt.Event, len(events)+4)
	for _, evt := range events {
		ch <- evt
	}
	return &eventfulSTTSession{events: ch}
}

func (m *eventfulSTTSession) Send(audio []byte) error {
	m.sent = append(m.sent, append([]byte(nil), audio...))
	return nil
}

func (m *eventfulSTTSession) Events() <-chan stt.Event { return m.events }
func (m *eventfulSTTSession) Close() error {
	m.closed = true
	close(m.events)
	return nil
}

func (m *eventfulSTTSession) push(evt stt.Event) {
	m.events <- evt
}

func TestHandleCommittedTranscriptExactlyOnce(t *testing.T) {
	var handedOff []string
	cs := &CallSession{ID: "sess-handoff", State: StateListening}
	cs.onCommittedTranscript = func(_ *CallSession, turn *Turn) {
		handedOff = append(handedOff, turn.CommittedText)
	}

	cs.handleCommittedTranscript("hello world")
	cs.handleCommittedTranscript("hello world")
	cs.handleCommittedTranscript("next utterance")

	assert.Equal(t, []string{"hello world", "next utterance"}, handedOff)
	assert.Equal(t, 2, cs.HandoffCount())
}

func TestHandleCommittedTranscriptDiscardsWhitespace(t *testing.T) {
	cs := &CallSession{ID: "sess-empty"}
	handoffs := 0
	cs.onCommittedTranscript = func(_ *CallSession, _ *Turn) {
		handoffs++
	}

	cs.handleCommittedTranscript("   ")
	cs.handleCommittedTranscript("\t\n")

	assert.Equal(t, 0, handoffs)
	assert.Equal(t, 0, cs.HandoffCount())
	assert.Nil(t, cs.CurrentTurn)
}

func TestHandleSTTPartialUpdatesTrackingOnly(t *testing.T) {
	cs := &CallSession{ID: "sess-partial"}
	handoffs := 0
	cs.onCommittedTranscript = func(_ *CallSession, _ *Turn) { handoffs++ }

	cs.handleSTTEvent(stt.Event{
		Kind:              stt.EventPartialTranscript,
		PartialTranscript: stt.PartialTranscript{Text: "hel"},
	})

	assert.Equal(t, "hel", cs.PartialText())
	assert.Equal(t, 0, handoffs)
}

func TestHandleSTTCommittedHandsOffOnce(t *testing.T) {
	var handedOff []string
	cs := &CallSession{ID: "sess-commit"}
	cs.onCommittedTranscript = func(_ *CallSession, turn *Turn) {
		handedOff = append(handedOff, turn.CommittedText)
	}

	cs.handleSTTEvent(stt.Event{
		Kind:                stt.EventCommittedTranscript,
		CommittedTranscript: stt.CommittedTranscript{Text: "book a flight"},
	})
	cs.handleSTTEvent(stt.Event{
		Kind:                stt.EventCommittedTranscript,
		CommittedTranscript: stt.CommittedTranscript{Text: "book a flight"},
	})

	assert.Equal(t, []string{"book a flight"}, handedOff)
	assert.Equal(t, 1, cs.HandoffCount())
}

func TestReconnectSTTReplacesSession(t *testing.T) {
	first := newEventfulSTTSession()
	second := newEventfulSTTSession()

	cs := &CallSession{
		ID:          "sess-reconnect",
		STT:         first,
		VoiceConfig: &VoiceConfig{ElevenLabsAPIKey: "key", Language: "en"},
		sttFactory: func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
			return second, nil
		},
	}

	require.NoError(t, cs.reconnectSTT(context.Background()))
	assert.Equal(t, second, cs.activeSTT())
	assert.True(t, first.closed)
}

func TestMediaRunnerSTTReconnectOnClosedEvent(t *testing.T) {
	first := newEventfulSTTSession()
	second := newEventfulSTTSession()

	cs := &CallSession{
		ID:          "sess-reconnect-loop",
		State:       StateListening,
		Conn:        &blockingConn{},
		STT:         first,
		VoiceConfig: &VoiceConfig{ElevenLabsAPIKey: "key", Language: "en"},
	}

	runner := NewMediaRunner(func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		return second, nil
	}, nil)
	runner.Start(cs)

	first.push(stt.Event{Kind: stt.EventClosed, Closed: stt.Closed{Err: assert.AnError}})

	deadline := time.After(2 * time.Second)
	for cs.activeSTT() != second {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for STT reconnect via event loop")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestForwardAudioToSTTUpsamples(t *testing.T) {
	sttSession := newEventfulSTTSession()
	cs := &CallSession{
		STT:         sttSession,
		VoiceConfig: &VoiceConfig{ElevenLabsAPIKey: "key"},
	}

	pcm8k := make([]byte, audiosocket.ExpectedAudioFrameSize)
	require.NoError(t, cs.forwardAudioToSTT(pcm8k))

	require.Len(t, sttSession.sent, 1)
	assert.Len(t, sttSession.sent[0], audiosocket.ExpectedAudioFrameSize*2)
}

type framePushConn struct {
	frames []audiosocket.Frame
	idx    int
}

func (c *framePushConn) ReadFrame() (audiosocket.Frame, error) {
	if c.idx >= len(c.frames) {
		return audiosocket.Frame{}, io.EOF
	}
	frame := c.frames[c.idx]
	c.idx++
	return frame, nil
}

func (c *framePushConn) WriteAudio([]byte) error { return nil }
func (c *framePushConn) Close() error          { return nil }

type blockingConn struct{}

func (c *blockingConn) ReadFrame() (audiosocket.Frame, error) {
	select {}
}

func (c *blockingConn) WriteAudio([]byte) error { return nil }
func (c *blockingConn) Close() error          { return nil }

