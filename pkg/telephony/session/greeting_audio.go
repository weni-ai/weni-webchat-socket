package session

import (
	"encoding/binary"
	"math"
	"os"
	"strings"
)

const greetingSampleRate = 8000

type greetingTone struct {
	freqHz       float64
	durationMs   int
	gapMs        int
}

// Builtin greeting: short melodic phrase (PCM 8 kHz, 16-bit LE mono).
// Optional via WWC_TELEPHONY_GREETING_AUDIO_PATH=builtin; empty uses ElevenLabs TTS.
var builtinGreetingTones = []greetingTone{
	{523.25, 200, 30},  // C5
	{659.25, 200, 30},  // E5
	{783.99, 200, 30},  // G5
	{1046.50, 400, 80}, // C6
	{880.00, 200, 30},  // A5
	{698.46, 200, 30},  // F5
	{523.25, 500, 0},   // C5
}

// BuiltinGreetingPCM returns a short audible melody for AudioSocket playback tests.
func BuiltinGreetingPCM() []byte {
	var pcm []byte
	for _, tone := range builtinGreetingTones {
		pcm = append(pcm, synthesizeTone(tone.freqHz, tone.durationMs)...)
		if tone.gapMs > 0 {
			pcm = append(pcm, silencePCM(tone.gapMs)...)
		}
	}
	return pcm
}

func loadGreetingPCM(path string) ([]byte, string, error) {
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "builtin", "embedded", "debug":
		return BuiltinGreetingPCM(), "builtin", nil
	default:
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		return data, path, nil
	}
}

func synthesizeTone(freqHz float64, durationMs int) []byte {
	numSamples := greetingSampleRate * durationMs / 1000
	pcm := make([]byte, numSamples*2)
	const amplitude = 0.65 * float64(1<<15)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(greetingSampleRate)
		envelope := toneEnvelope(i, numSamples)
		sample := int16(amplitude * envelope * math.Sin(2*math.Pi*freqHz*t))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(sample))
	}
	return pcm
}

func toneEnvelope(sampleIndex, totalSamples int) float64 {
	fade := greetingSampleRate / 100 // 10 ms fade in/out
	if fade < 1 {
		fade = 1
	}
	switch {
	case sampleIndex < fade:
		return float64(sampleIndex) / float64(fade)
	case sampleIndex > totalSamples-fade:
		return float64(totalSamples-sampleIndex) / float64(fade)
	default:
		return 1
	}
}

func silencePCM(durationMs int) []byte {
	numSamples := greetingSampleRate * durationMs / 1000
	return make([]byte, numSamples*2)
}
