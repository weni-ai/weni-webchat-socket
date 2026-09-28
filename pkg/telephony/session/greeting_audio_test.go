package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type emptyTTSClient struct{}

func (emptyTTSClient) Synthesize(context.Context, string, string, string) (<-chan []byte, error) {
	ch := make(chan []byte)
	close(ch)
	return ch, nil
}

func TestBuiltinGreetingPCMIsAudibleSized(t *testing.T) {
	pcm := BuiltinGreetingPCM()
	require.NotEmpty(t, pcm)
	assert.Equal(t, 0, len(pcm)%2, "PCM must be 16-bit samples")
	assert.Greater(t, len(pcm), greetingSampleRate*2, "greeting should be longer than 1 second")
}

func TestLoadGreetingPCMFromBuiltinAlias(t *testing.T) {
	pcm, source, err := loadGreetingPCM("builtin")
	require.NoError(t, err)
	assert.Equal(t, "builtin", source)
	assert.NotEmpty(t, pcm)
}

func TestSetupRunnerPlaysBuiltinGreetingWithoutTTS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.pcm")
	customPCM := BuiltinGreetingPCM()
	require.NoError(t, os.WriteFile(path, customPCM, 0o644))

	cfg := config.Get()
	original := cfg.Telephony.GreetingAudioPath
	cfg.Telephony.GreetingAudioPath = path
	defer func() { cfg.Telephony.GreetingAudioPath = original }()

	conn := &recordingAudioConn{}
	cs := &CallSession{
		ID: "sess-builtin-greeting",
		Conn: conn,
		VoiceConfig: &VoiceConfig{Language: "en"},
	}
	cs.ensureAudioKeepalive()

	runner := NewSetupRunner(nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, runner.playGreeting(context.Background(), cs, "ignored"))

	assert.Greater(t, conn.writtenBytes(), 0)
}

func TestPlaySpokenTextEmptyAudioIsError(t *testing.T) {
	cs := &CallSession{
		ID:   "sess-empty-tts",
		Conn: &recordingAudioConn{},
		VoiceConfig: &VoiceConfig{
			VoiceID:  "voice-1",
			Language: "en",
		},
	}
	runner := NewSetupRunner(nil, nil, func(*VoiceConfig) tts.TTSStreamClient {
		return emptyTTSClient{}
	}, nil, nil, nil, nil)

	err := runner.playSpokenText(context.Background(), cs, "The voice assistant is ready")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no audio")
	assert.Equal(t, 0, cs.Conn.(*recordingAudioConn).writtenBytes())
}

type recordingAudioConn struct {
	frames [][]byte
}

func (c *recordingAudioConn) ReadFrame() (audiosocket.Frame, error) {
	return audiosocket.Frame{}, nil
}

func (c *recordingAudioConn) WriteAudio(audio []byte) error {
	c.frames = append(c.frames, append([]byte(nil), audio...))
	return nil
}

func (c *recordingAudioConn) Close() error { return nil }

func (c *recordingAudioConn) writtenBytes() int {
	total := 0
	for _, frame := range c.frames {
		total += len(frame)
	}
	return total
}
