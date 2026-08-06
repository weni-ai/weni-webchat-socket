package session

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVoiceConfigConfiguredLanguage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("test-key", nil)
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("pt", nil)

	cfg, err := ResolveVoiceConfig(mockFlows, "ch-1")
	require.NoError(t, err)
	assert.Equal(t, "pt", cfg.Language)
}

func TestResolveVoiceConfigEmptyLanguageDefaultsToEnglish(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockFlows := flows.NewMockIClient(ctrl)
	mockFlows.EXPECT().GetElevenLabsAPIKey("ch-1").Return("test-key", nil)
	mockFlows.EXPECT().GetChannelProjectLanguage("ch-1").Return("", nil)

	cfg, err := ResolveVoiceConfig(mockFlows, "ch-1")
	require.NoError(t, err)
	assert.Equal(t, DefaultLanguage, cfg.Language)
}

func TestNormalizeLanguageCode(t *testing.T) {
	assert.Equal(t, DefaultLanguage, NormalizeLanguageCode(""))
	assert.Equal(t, DefaultLanguage, NormalizeLanguageCode("   "))
	assert.Equal(t, "pt", NormalizeLanguageCode("PT"))
	assert.Equal(t, "pt-br", NormalizeLanguageCode(" PT-BR "))
}

func TestIsUnsupportedLanguageError(t *testing.T) {
	assert.False(t, IsUnsupportedLanguageError(nil))
	assert.False(t, IsUnsupportedLanguageError(fmt.Errorf("stt: dial websocket: connection refused")))
	assert.True(t, IsUnsupportedLanguageError(fmt.Errorf("stt: unsupported language_code xx")))
	assert.True(t, IsUnsupportedLanguageError(fmt.Errorf("tts: invalid language parameter")))
}

func TestOpenSTTSessionFallsBackToEnglishOnUnsupportedLanguage(t *testing.T) {
	attempts := 0
	factory := func(_ context.Context, cfg *VoiceConfig) (stt.STTSession, error) {
		attempts++
		if cfg.Language == "xx" {
			return nil, fmt.Errorf("stt: unsupported language_code xx")
		}
		if cfg.Language == DefaultLanguage {
			return &mockSTTSession{}, nil
		}
		return nil, fmt.Errorf("unexpected language %q", cfg.Language)
	}

	cfg := &VoiceConfig{
		ElevenLabsAPIKey: "test-key",
		Language:         "xx",
	}

	session, err := OpenSTTSession(context.Background(), factory, cfg)
	require.NoError(t, err)
	require.NotNil(t, session)
	assert.Equal(t, DefaultLanguage, cfg.Language)
	assert.Equal(t, 2, attempts)
}

func TestOpenSTTSessionDoesNotFallbackOnNonLanguageErrors(t *testing.T) {
	factory := func(_ context.Context, _ *VoiceConfig) (stt.STTSession, error) {
		return nil, fmt.Errorf("stt: auth_error: invalid api key")
	}

	cfg := &VoiceConfig{
		ElevenLabsAPIKey: "bad-key",
		Language:         "pt",
	}

	_, err := OpenSTTSession(context.Background(), factory, cfg)
	require.Error(t, err)
	assert.Equal(t, "pt", cfg.Language)
}
