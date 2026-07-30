package session

import (
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	log "github.com/sirupsen/logrus"
)

// startTTSStream prepares a fresh batcher and writer for an agent response stream.
func (cs *CallSession) startTTSStream(msgID string) {
	cs.stopTTSStream()

	if cs.VoiceConfig == nil || cs.ttsFactory == nil {
		log.WithField("session_id", cs.ID).Warn("telephony: TTS not configured for stream")
		return
	}

	cs.grpcMu.Lock()
	cs.CurrentTurn = &Turn{MsgID: msgID, StartedAt: time.Now()}
	cs.grpcMu.Unlock()

	client := cs.ttsFactory(cs.VoiceConfig)
	batcher := tts.NewTTSBatcher(
		client,
		cs.VoiceConfig.VoiceID,
		cs.VoiceConfig.Language,
		cs.VoiceConfig.TTSMinBatchChars,
	)

	cs.ttsWriterMu.Lock()
	cs.ttsBatcher = batcher
	done := make(chan struct{})
	cs.ttsWriterDone = done
	cs.ttsWriterMu.Unlock()

	go cs.runTTSWriter(batcher, done)

	if cs.CurrentState() == StateListening {
		_ = cs.transition(StateProcessing)
	}
}

func (cs *CallSession) appendTTSDelta(delta string) {
	cs.ttsWriterMu.Lock()
	batcher := cs.ttsBatcher
	cs.ttsWriterMu.Unlock()
	if batcher != nil {
		batcher.Append(delta)
	}
}

func (cs *CallSession) flushTTSStream() {
	cs.ttsWriterMu.Lock()
	batcher := cs.ttsBatcher
	cs.ttsWriterMu.Unlock()
	if batcher != nil {
		batcher.Flush(true)
	}
}

func (cs *CallSession) stopTTSStream() {
	cs.ttsWriterMu.Lock()
	batcher := cs.ttsBatcher
	done := cs.ttsWriterDone
	cs.ttsBatcher = nil
	cs.ttsWriterDone = nil
	cs.ttsWriterMu.Unlock()

	if batcher != nil {
		batcher.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			log.WithField("session_id", cs.ID).Warn("telephony: timed out waiting for TTS writer")
		}
	}
}

func (cs *CallSession) runTTSWriter(batcher *tts.TTSBatcher, done chan struct{}) {
	defer close(done)

	var (
		speaking     bool
		lastBatchIdx = -1
		gapless      = true
		batchMarkers []int
		batchStart   time.Time
	)

	for chunk := range batcher.Output() {
		if chunk.StreamEnd {
			if speaking {
				_ = cs.transition(StateListening)
			}
			cs.recordGaplessPlayback(gapless, batchMarkers)
			return
		}

		if len(chunk.PCM) > 0 {
			batchIdx := int(chunk.PCM[0])

			if !speaking {
				if err := cs.transition(StateSpeaking); err != nil {
					log.WithFields(log.Fields{
						"session_id": cs.ID,
					}).WithError(err).Warn("telephony: failed to transition to speaking")
				}
				speaking = true
			}

			if lastBatchIdx >= 0 && batchIdx < lastBatchIdx {
				gapless = false
			}
			if batchIdx > lastBatchIdx {
				if lastBatchIdx >= 0 && !batchStart.IsZero() && time.Since(batchStart) > 100*time.Millisecond {
					gapless = false
				}
				lastBatchIdx = batchIdx
				batchStart = time.Now()
				batchMarkers = append(batchMarkers, batchIdx)
			}

			if cs.Conn != nil {
				if err := writeAudioFrames(cs.Conn, chunk.PCM); err != nil {
					log.WithFields(log.Fields{
						"session_id": cs.ID,
					}).WithError(err).Warn("telephony: failed to write TTS audio")
					return
				}
			}
		}

		if chunk.BatchEnd {
			if cs.metrics != nil && !batchStart.IsZero() {
				cs.metrics.ObserveTTSBatchDuration(time.Since(batchStart).Seconds())
			}
			cs.grpcMu.Lock()
			if cs.CurrentTurn != nil {
				cs.CurrentTurn.BatchesIssued++
			}
			cs.grpcMu.Unlock()
		}
	}

	if speaking {
		_ = cs.transition(StateListening)
	}
}

func (cs *CallSession) recordGaplessPlayback(gapless bool, batchMarkers []int) {
	cs.ttsPlaybackMu.Lock()
	cs.lastGaplessPlayback = gapless
	cs.lastBatchMarkers = append([]int(nil), batchMarkers...)
	cs.ttsPlaybackMu.Unlock()
}

// LastGaplessPlayback reports whether the last TTS stream had gapless batch sequencing (for tests).
func (cs *CallSession) LastGaplessPlayback() bool {
	cs.ttsPlaybackMu.Lock()
	defer cs.ttsPlaybackMu.Unlock()
	return cs.lastGaplessPlayback
}

// LastBatchMarkers returns batch index markers from the last playback (for tests).
func (cs *CallSession) LastBatchMarkers() []int {
	cs.ttsPlaybackMu.Lock()
	defer cs.ttsPlaybackMu.Unlock()
	return append([]int(nil), cs.lastBatchMarkers...)
}
