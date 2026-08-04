package session

import (
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/flows"
)

// VoiceConfig holds resolved voice/STT/TTS settings for a CallSession.
type VoiceConfig struct {
	ElevenLabsAPIKey string
	VoiceID          string
	Language         string
	STTModelID       string
	TTSModelID       string
	VADSilenceMs     int64
	TTSMinBatchChars int64
	MaxConcurrency   int64
}

// ResolveVoiceConfig loads channel-specific overrides and config defaults.
func ResolveVoiceConfig(flowsClient flows.IClient, channelUUID string) (*VoiceConfig, error) {
	telephonyCfg := config.Get().Telephony

	apiKey, err := flowsClient.GetElevenLabsAPIKey(channelUUID)
	if err != nil {
		return nil, err
	}

	language, err := flowsClient.GetChannelProjectLanguage(channelUUID)
	if err != nil {
		return nil, err
	}
	language = NormalizeLanguageCode(language)

	return &VoiceConfig{
		ElevenLabsAPIKey: apiKey,
		VoiceID:          telephonyCfg.VoiceID,
		Language:         language,
		STTModelID:       telephonyCfg.STTModelID,
		TTSModelID:       telephonyCfg.TTSModelID,
		VADSilenceMs:     telephonyCfg.VADSilenceMs,
		TTSMinBatchChars: telephonyCfg.TTSMinBatchChars,
		MaxConcurrency:   telephonyCfg.MaxConcurrentCalls,
	}, nil
}
