package session

import "errors"

// ErrorCode identifies a telephony voice error category.
type ErrorCode string

const (
	ErrSTTUnavailable    ErrorCode = "stt_unavailable"
	ErrChannelUnresolved ErrorCode = "channel_unresolved"
	ErrAgentUnavailable  ErrorCode = "agent_unavailable"
	ErrTTSBatchFailed    ErrorCode = "tts_batch_failed"
	ErrMediaError        ErrorCode = "media_error"
)

// VoiceError describes a telephony failure with recovery semantics.
type VoiceError struct {
	Code        ErrorCode
	Message     string
	SpokenKey   string
	Recoverable bool
}

func (e *VoiceError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Recoverable reports whether err is a recoverable VoiceError.
func Recoverable(err error) bool {
	var ve *VoiceError
	if errors.As(err, &ve) {
		return ve.Recoverable
	}
	return false
}
