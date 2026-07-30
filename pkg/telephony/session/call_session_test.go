package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/stretchr/testify/assert"
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
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, func(sessionID string) {
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
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, func(sessionID string) {
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
	runner := NewSetupRunner(mockFlows, sttFactory, ttsFactory, nil, func(sessionID string) {
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
