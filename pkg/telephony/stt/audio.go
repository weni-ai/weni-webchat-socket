package stt

import (
	"encoding/binary"
	"fmt"
)

// Upsample8kTo16k converts PCM 16-bit LE mono 8 kHz audio to 16 kHz using
// linear interpolation between consecutive samples.
func Upsample8kTo16k(pcm8k []byte) ([]byte, error) {
	if len(pcm8k) == 0 {
		return nil, nil
	}
	if len(pcm8k)%2 != 0 {
		return nil, fmt.Errorf("stt: pcm8k length must be even, got %d", len(pcm8k))
	}

	numSamples := len(pcm8k) / 2
	out := make([]byte, numSamples*4)

	for i := 0; i < numSamples; i++ {
		sample := int16(binary.LittleEndian.Uint16(pcm8k[i*2:]))

		var next int16
		if i+1 < numSamples {
			next = int16(binary.LittleEndian.Uint16(pcm8k[(i+1)*2:]))
		} else {
			next = sample
		}

		mid := int16((int32(sample) + int32(next)) / 2)
		binary.LittleEndian.PutUint16(out[i*4:], uint16(sample))
		binary.LittleEndian.PutUint16(out[i*4+2:], uint16(mid))
	}

	return out, nil
}
