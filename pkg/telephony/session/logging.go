package session

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// logFields returns structured log fields consistent with pkg/grpc and pkg/websocket conventions.
func (cs *CallSession) logFields() log.Fields {
	return log.Fields{
		"session_id":   cs.ID,
		"channel_uuid": cs.ChannelUUID,
		"project_uuid": cs.ProjectUUID,
		"contact_urn":  cs.ContactURN,
		"state":        cs.CurrentState(),
	}
}

func voiceConfigLogFields(cfg *VoiceConfig) log.Fields {
	if cfg == nil {
		return log.Fields{"voice_config": "(nil)"}
	}
	return log.Fields{
		"elevenlabs_api_key": maskSecret(cfg.ElevenLabsAPIKey),
		"voice_id":           cfg.VoiceID,
		"language":           cfg.Language,
		"stt_model_id":       cfg.STTModelID,
		"tts_model_id":       cfg.TTSModelID,
	}
}

func maskSecret(value string) string {
	if value == "" {
		return "(empty)"
	}
	if len(value) <= 4 {
		return "****"
	}
	return fmt.Sprintf("...%s (len=%d)", value[len(value)-4:], len(value))
}
