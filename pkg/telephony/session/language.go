package session

import (
	"context"
	"strings"

	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	log "github.com/sirupsen/logrus"
)

const DefaultLanguage = "en"

// NormalizeLanguageCode returns an ElevenLabs-compatible ISO 639-1 language code.
// BCP47 locales such as "en-us" are reduced to their base language ("en").
func NormalizeLanguageCode(language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "" {
		return DefaultLanguage
	}
	lang = strings.ReplaceAll(lang, "_", "-")
	if i := strings.IndexByte(lang, '-'); i > 0 {
		lang = lang[:i]
	}
	return lang
}

// IsUnsupportedLanguageError reports whether err indicates the language code was rejected by STT/TTS.
func IsUnsupportedLanguageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "language") ||
		strings.Contains(msg, "unsupported") ||
		strings.Contains(msg, "invalid_language")
}

// OpenSTTSession opens an STT session, falling back to DefaultLanguage when the configured language is rejected.
func OpenSTTSession(ctx context.Context, factory STTSessionFactory, cfg *VoiceConfig) (stt.STTSession, error) {
	session, err := factory(ctx, cfg)
	if err == nil {
		return session, nil
	}
	if cfg.Language == DefaultLanguage || !IsUnsupportedLanguageError(err) {
		return nil, err
	}

	log.WithFields(log.Fields{
		"language": cfg.Language,
	}).WithError(err).Warn("telephony: unsupported language, falling back to English")

	cfg.Language = DefaultLanguage
	return factory(ctx, cfg)
}
