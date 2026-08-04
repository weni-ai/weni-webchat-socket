package session

import (
	"context"
	"fmt"
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
	assert.NotEmpty(t, conn.WrittenLen())
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
	assert.NotEmpty(t, conn.WrittenLen())

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

type countingTTSClient struct {
	mu    sync.Mutex
	calls []string
}

func (c *countingTTSClient) Synthesize(_ context.Context, text, _, _ string) (<-chan []byte, error) {
	c.mu.Lock()
	idx := len(c.calls)
	c.calls = append(c.calls, text)
	c.mu.Unlock()

	ch := make(chan []byte, 1)
	ch <- []byte{byte(idx), 0x01, 0x02, 0x03}
	close(ch)
	return ch, nil
}

func (c *countingTTSClient) Calls() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.calls...)
}

func TestThreeSentenceDeltaStreamEndToEnd(t *testing.T) {
	countingClient := &countingTTSClient{}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-tts-e2e",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return countingClient
		},
	}

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-1"}`))
	cs.handleGRPCPayload([]byte(`{"v":"First sentence. ","seq":1}`))
	cs.handleGRPCPayload([]byte(`{"v":"Second sentence. ","seq":2}`))
	cs.handleGRPCPayload([]byte(`{"v":"Third sentence.","seq":3}`))
	cs.handleGRPCPayload([]byte(`{"type":"stream_end","id":"msg-1"}`))

	deadline := time.After(3 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out, state=%s calls=%v", cs.CurrentState(), countingClient.Calls())
		case <-time.After(20 * time.Millisecond):
		}
	}

	calls := countingClient.Calls()
	assert.GreaterOrEqual(t, len(calls), 3)
	assert.LessOrEqual(t, len(calls), 4)
	assert.Equal(t, []string{"First sentence.", "Second sentence.", "Third sentence."}, calls)
	assert.True(t, cs.LastGaplessPlayback())
	require.Len(t, cs.LastBatchMarkers(), 3)
	assert.Equal(t, []int{0, 1, 2}, cs.LastBatchMarkers())
	assert.NotEmpty(t, conn.WrittenLen())
}

func TestTTSBatchFailureReturnsToListening(t *testing.T) {
	failingClient := &recordingTTSClientSession{
		errOn: map[int]error{0: assert.AnError},
	}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-tts-fail",
		State: StateProcessing,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return failingClient
		},
	}

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-2"}`))
	cs.handleGRPCPayload([]byte(`{"v":"Broken batch. ","seq":1}`))
	cs.handleGRPCPayload([]byte(`{"v":"Recovery sentence.","seq":2}`))
	cs.handleGRPCPayload([]byte(`{"type":"stream_end","id":"msg-2"}`))

	deadline := time.After(3 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out, state=%s", cs.CurrentState())
		case <-time.After(20 * time.Millisecond):
		}
	}

	assert.Equal(t, []string{"Broken batch.", "Recovery sentence."}, failingClient.Calls())
}

type recordingTTSClientSession struct {
	mu    sync.Mutex
	calls []string
	errOn map[int]error
}

func (r *recordingTTSClientSession) Synthesize(_ context.Context, text, _, _ string) (<-chan []byte, error) {
	r.mu.Lock()
	idx := len(r.calls)
	r.calls = append(r.calls, text)
	err := r.errOn[idx]
	r.mu.Unlock()
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- []byte{byte(idx), 0x01}
	close(ch)
	return ch, nil
}

func (r *recordingTTSClientSession) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

type slowCancellableTTSClient struct {
	mu        sync.Mutex
	cancelled bool
}

func (c *slowCancellableTTSClient) Synthesize(ctx context.Context, text, _, _ string) (<-chan []byte, error) {
	ch := make(chan []byte, 8)
	go func() {
		defer close(ch)
		select {
		case ch <- []byte{0, 0x01, 0x02, 0x03}:
		case <-ctx.Done():
			c.markCancelled()
			return
		}
		select {
		case <-ctx.Done():
			c.markCancelled()
		case <-time.After(5 * time.Second):
			ch <- []byte{0, 0x04, 0x05, 0x06}
		}
	}()
	return ch, nil
}

func (c *slowCancellableTTSClient) markCancelled() {
	c.mu.Lock()
	c.cancelled = true
	c.mu.Unlock()
}

func (c *slowCancellableTTSClient) WasCancelled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelled
}

func TestBargeInStopsWriterAndCancelsTTSWithinBudget(t *testing.T) {
	slowClient := &slowCancellableTTSClient{}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-bargein-stop",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return slowClient
		},
	}
	cs.ensureBargeIn()

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-barge"}`))
	cs.handleGRPCPayload([]byte(`{"v":"This is a long agent response without punctuation to keep synthesis in flight","seq":1}`))

	deadline := time.After(2 * time.Second)
	for cs.CurrentState() != StateSpeaking {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for speaking, state=%s", cs.CurrentState())
		case <-time.After(10 * time.Millisecond):
		}
	}

	start := time.Now()
	cs.handleSTTEvent(stt.Event{
		Kind:              stt.EventPartialTranscript,
		PartialTranscript: stt.PartialTranscript{Text: "wait"},
	})

	deadline = time.After(2 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for listening after barge-in, state=%s", cs.CurrentState())
		case <-time.After(5 * time.Millisecond):
		}
	}

	elapsed := time.Since(start)
	assert.Less(t, elapsed, 300*time.Millisecond, "barge-in should stop playback within 300ms")
	assert.True(t, slowClient.WasCancelled(), "in-flight TTS synthesis should be cancelled")
}

func TestBargeInCommittedTranscriptStartsNewTurn(t *testing.T) {
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-bargein-turn",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return &countingTTSClient{}
		},
	}
	cs.ensureBargeIn()

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-turn"}`))
	cs.handleGRPCPayload([]byte(`{"v":"Agent speaking now. ","seq":1}`))

	deadline := time.After(2 * time.Second)
	for cs.CurrentState() != StateSpeaking {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for speaking, state=%s", cs.CurrentState())
		case <-time.After(10 * time.Millisecond):
		}
	}

	cs.grpcMu.Lock()
	priorTurn := cs.CurrentTurn
	cs.grpcMu.Unlock()
	require.NotNil(t, priorTurn)

	cs.handleSTTEvent(stt.Event{
		Kind:              stt.EventPartialTranscript,
		PartialTranscript: stt.PartialTranscript{Text: "stop"},
	})

	assert.True(t, priorTurn.Interrupted)

	cs.handleCommittedTranscript("new caller question")
	require.NotNil(t, cs.CurrentTurn)
	assert.False(t, cs.CurrentTurn.Interrupted)
	assert.NotSame(t, priorTurn, cs.CurrentTurn)
	assert.Equal(t, "new caller question", cs.CurrentTurn.CommittedText)
}

func TestBargeInLatencyUnder300ms(t *testing.T) {
	metrics, err := NewSessionMetrics(nil)
	require.NoError(t, err)

	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-bargein-latency",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		metrics: metrics,
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return &countingTTSClient{}
		},
	}
	cs.ensureBargeIn()

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-latency"}`))
	cs.handleGRPCPayload([]byte(`{"v":"Speaking sentence one. ","seq":1}`))
	cs.handleGRPCPayload([]byte(`{"v":"Speaking sentence two.","seq":2}`))

	deadline := time.After(2 * time.Second)
	for cs.CurrentState() != StateSpeaking {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for speaking, state=%s", cs.CurrentState())
		case <-time.After(10 * time.Millisecond):
		}
	}

	cs.handleSTTEvent(stt.Event{
		Kind:              stt.EventPartialTranscript,
		PartialTranscript: stt.PartialTranscript{Text: "hello"},
	})

	deadline = time.After(2 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for listening after barge-in, state=%s", cs.CurrentState())
		case <-time.After(5 * time.Millisecond):
		}
	}

	latency := cs.LastBargeInLatency()
	assert.Greater(t, latency, time.Duration(0))
	assert.Less(t, latency, 300*time.Millisecond)
}

type languageTrackingTTSClient struct {
	mu        sync.Mutex
	languages []string
}

func (c *languageTrackingTTSClient) Synthesize(_ context.Context, text, _, language string) (<-chan []byte, error) {
	c.mu.Lock()
	c.languages = append(c.languages, language)
	c.mu.Unlock()

	ch := make(chan []byte, 1)
	ch <- []byte{0, 0x01, 0x02, 0x03}
	close(ch)
	return ch, nil
}

func (c *languageTrackingTTSClient) Languages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.languages...)
}

func TestCallSessionUpdateLanguageAppliedOnReconnectAndTTS(t *testing.T) {
	var sttLanguages []string
	sttFactory := func(_ context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		sttLanguages = append(sttLanguages, cfg.Language)
		return &mockSTTSession{}, nil
	}

	ttsClient := &languageTrackingTTSClient{}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-lang-update",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			ElevenLabsAPIKey: "test-key",
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		Language:   "en",
		sttFactory: sttFactory,
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return ttsClient
		},
	}

	require.NoError(t, cs.reconnectSTT(context.Background()))
	cs.UpdateLanguage("pt")
	require.NoError(t, cs.reconnectSTT(context.Background()))

	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-lang"}`))
	cs.handleGRPCPayload([]byte(`{"v":"Olá. ","seq":1}`))
	cs.handleGRPCPayload([]byte(`{"v":"Tudo bem?","seq":2}`))

	deadline := time.After(2 * time.Second)
	for cs.CurrentState() != StateSpeaking && cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for TTS playback, state=%s", cs.CurrentState())
		case <-time.After(10 * time.Millisecond):
		}
	}

	assert.Equal(t, []string{"en", "pt"}, sttLanguages)
	require.NotEmpty(t, ttsClient.Languages())
	for _, lang := range ttsClient.Languages() {
		assert.Equal(t, "pt", lang)
	}
}

const listeningTurnaroundBudget = 500 * time.Millisecond

func TestFlushFinalReturnsToListeningWithinTurnaround(t *testing.T) {
	countingClient := &countingTTSClient{}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-turnaround",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return countingClient
		},
	}

	start := time.Now()
	cs.handleGRPCPayload([]byte(`{"type":"stream_start","id":"msg-turnaround"}`))
	cs.handleGRPCPayload([]byte(`{"v":"Done speaking now. ","seq":1}`))
	cs.handleGRPCPayload([]byte(`{"type":"stream_end","id":"msg-turnaround"}`))

	deadline := time.After(2 * time.Second)
	for cs.CurrentState() != StateListening {
		select {
		case <-deadline:
			t.Fatalf("timed out, state=%s", cs.CurrentState())
		case <-time.After(5 * time.Millisecond):
		}
	}

	elapsed := time.Since(start)
	assert.Equal(t, StateListening, cs.CurrentState())
	assert.Less(t, elapsed, listeningTurnaroundBudget,
		"session should return to listening within turnaround budget after final flush")
}

func TestTwelveTurnLongevityStableProcessing(t *testing.T) {
	countingClient := &countingTTSClient{}
	conn := &mockAudioConn{}
	cs := &CallSession{
		ID:    "sess-longevity",
		State: StateListening,
		Conn:  conn,
		VoiceConfig: &VoiceConfig{
			VoiceID:          "voice-1",
			Language:         "en",
			TTSMinBatchChars: 40,
		},
		ttsFactory: func(_ *VoiceConfig) tts.TTSStreamClient {
			return countingClient
		},
	}

	const turns = 12
	const perTurnBudget = time.Second
	durations := make([]time.Duration, 0, turns)

	for i := 0; i < turns; i++ {
		start := time.Now()
		msgID := fmt.Sprintf("msg-longevity-%d", i)
		cs.handleGRPCPayload([]byte(fmt.Sprintf(`{"type":"stream_start","id":"%s"}`, msgID)))
		cs.handleGRPCPayload([]byte(fmt.Sprintf(`{"v":"Turn %d response. ","seq":1}`, i)))
		cs.handleGRPCPayload([]byte(fmt.Sprintf(`{"type":"stream_end","id":"%s"}`, msgID)))

		deadline := time.After(2 * time.Second)
		for cs.CurrentState() != StateListening {
			select {
			case <-deadline:
				t.Fatalf("turn %d timed out in state %s", i, cs.CurrentState())
			case <-time.After(5 * time.Millisecond):
			}
		}

		elapsed := time.Since(start)
		durations = append(durations, elapsed)
		assert.Less(t, elapsed, perTurnBudget, "turn %d exceeded per-turn budget", i)
	}

	firstAvg := averageDuration(durations[:3])
	lastAvg := averageDuration(durations[len(durations)-3:])
	assert.LessOrEqual(t, lastAvg, firstAvg*2+50*time.Millisecond,
		"late turns should not degrade beyond 2x early-turn average (durations=%v)", durations)
	assert.Len(t, countingClient.Calls(), turns)
}

func averageDuration(d []time.Duration) time.Duration {
	if len(d) == 0 {
		return 0
	}
	var total time.Duration
	for _, v := range d {
		total += v
	}
	return total / time.Duration(len(d))
}

