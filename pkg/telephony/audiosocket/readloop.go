package audiosocket

import (
	"io"

	log "github.com/sirupsen/logrus"
)

const (
	// ExpectedAudioFrameSize is the nominal 20 ms PCM frame at 8 kHz (160 samples × 2 bytes).
	ExpectedAudioFrameSize = 320
	// AudioFrameTolerance allows slight length variation in incoming frames.
	AudioFrameTolerance = 16
)

// AudioFrameConsumer receives validated 8 kHz PCM audio payloads.
type AudioFrameConsumer func(pcm []byte)

// HangupHandler is invoked when a hangup frame is received.
type HangupHandler func()

// ReadLoopConfig configures the AudioSocket read loop.
type ReadLoopConfig struct {
	OnAudio  AudioFrameConsumer
	OnHangup HangupHandler
}

// RunReadLoop reads AudioSocket frames until the connection closes or a hangup is received.
// Malformed audio frames are dropped and logged without terminating the session.
func RunReadLoop(conn AudioSocketConn, cfg ReadLoopConfig) {
	for {
		frame, err := conn.ReadFrame()
		if err != nil {
			if err != io.EOF {
				log.WithError(err).Debug("audiosocket: read loop ended")
			}
			return
		}

		switch frame.Kind {
		case KindHangup:
			if cfg.OnHangup != nil {
				cfg.OnHangup()
			}
			return
		case KindDTMF:
			continue
		case KindAudio:
			if !ValidAudioFrameLength(len(frame.Payload)) {
				log.WithField("length", len(frame.Payload)).Warn("audiosocket: dropping malformed audio frame")
				continue
			}
			if cfg.OnAudio != nil {
				pcm := append([]byte(nil), frame.Payload...)
				cfg.OnAudio(pcm)
			}
		case KindError:
			log.WithField("payload_len", len(frame.Payload)).Warn("audiosocket: error frame received")
		}
	}
}

// ValidAudioFrameLength reports whether len is within the expected 320-byte ± tolerance range.
func ValidAudioFrameLength(length int) bool {
	minLen := ExpectedAudioFrameSize - AudioFrameTolerance
	maxLen := ExpectedAudioFrameSize + AudioFrameTolerance
	return length >= minLen && length <= maxLen
}
