package session

import (
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/flows"
	log "github.com/sirupsen/logrus"
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
		log.WithFields(log.Fields{
			"channel_uuid": channelUUID,
			"step":         "get_elevenlabs_api_key",
		}).WithError(err).Error("telephony: failed to resolve voice config")
		return nil, err
	}

	language, err := flowsClient.GetChannelProjectLanguage(channelUUID)
	if err != nil {
		log.WithFields(log.Fields{
			"channel_uuid":       channelUUID,
			"step":               "get_channel_project_language",
			"elevenlabs_api_key": maskSecret(apiKey),
		}).WithError(err).Error("telephony: failed to resolve voice config")
		return nil, err
	}
	language = NormalizeLanguageCode(language)

	cfg := &VoiceConfig{
		ElevenLabsAPIKey: apiKey,
		VoiceID:          telephonyCfg.VoiceID,
		Language:         language,
		STTModelID:       telephonyCfg.STTModelID,
		TTSModelID:       telephonyCfg.TTSModelID,
		VADSilenceMs:     telephonyCfg.VADSilenceMs,
		TTSMinBatchChars: telephonyCfg.TTSMinBatchChars,
		MaxConcurrency:   telephonyCfg.MaxConcurrentCalls,
	}

	log.WithFields(log.Fields{
		"channel_uuid": channelUUID,
		"step":         "voice_config_resolved",
	}).WithFields(voiceConfigLogFields(cfg)).Info("telephony: voice config resolved")

	return cfg, nil
}
