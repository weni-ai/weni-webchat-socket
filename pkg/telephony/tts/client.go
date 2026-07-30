package tts

import "context"

// TTSStreamClient abstracts a gateway-side ElevenLabs streaming TTS client.
type TTSStreamClient interface {
	Synthesize(ctx context.Context, text, voiceID, language string) (<-chan []byte, error)
}
