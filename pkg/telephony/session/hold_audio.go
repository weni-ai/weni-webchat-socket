package session

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
)

const holdAudioFrameSize = 320

// StartHoldAudioLoop plays hold audio in a loop while the session remains Queued.
// It reads holdAudioPath (WWC_TELEPHONY_HOLD_AUDIO_PATH) and writes fixed-size PCM
// frames over the AudioSocket connection until the session is promoted or ends.
func (cs *CallSession) StartHoldAudioLoop(holdAudioPath string) {
	if holdAudioPath == "" {
		log.WithField("session_id", cs.ID).Warn("hold audio path not configured")
		return
	}

	cs.holdAudioMu.Lock()
	if cs.holdAudioRunning {
		cs.holdAudioMu.Unlock()
		return
	}
	cs.holdAudioRunning = true
	cs.holdAudioMu.Unlock()

	go cs.runHoldAudioLoop(holdAudioPath)
}

func (cs *CallSession) runHoldAudioLoop(holdAudioPath string) {
	defer func() {
		cs.holdAudioMu.Lock()
		cs.holdAudioRunning = false
		cs.holdAudioMu.Unlock()
	}()

	data, err := os.ReadFile(holdAudioPath)
	if err != nil {
		log.WithFields(log.Fields{
			"session_id": cs.ID,
			"path":       holdAudioPath,
		}).WithError(err).Error("failed to read hold audio")
		return
	}

	for cs.CurrentState() == StateQueued {
		for offset := 0; offset < len(data) && cs.CurrentState() == StateQueued; offset += holdAudioFrameSize {
			end := offset + holdAudioFrameSize
			if end > len(data) {
				end = len(data)
			}
			if cs.Conn == nil {
				return
			}
			if err := cs.Conn.WriteAudio(data[offset:end]); err != nil {
				log.WithFields(log.Fields{
					"session_id": cs.ID,
				}).WithError(err).Debug("hold audio write stopped")
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
