package session

import (
	"context"
	"strings"
	"time"

	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	log "github.com/sirupsen/logrus"
)

const audioForwardBufferSize = 32

// CommittedTranscriptHandler is invoked exactly once per non-empty committed transcript.
// Phase 5 wires this to PostTranscript; Phase 4 tests spy on it directly.
type CommittedTranscriptHandler func(cs *CallSession, turn *Turn)

// MediaRunner streams AudioSocket audio to STT and processes STT events.
type MediaRunner struct {
	sttFactory  STTSessionFactory
	onCommitted CommittedTranscriptHandler
	onHangup    func(cs *CallSession)
}

// NewMediaRunner creates a MediaRunner with the given dependencies.
func NewMediaRunner(sttFactory STTSessionFactory, onCommitted CommittedTranscriptHandler) *MediaRunner {
	return &MediaRunner{
		sttFactory:  sttFactory,
		onCommitted: onCommitted,
	}
}

// SetHangupHandler wires the callback invoked when a hangup frame is received.
func (r *MediaRunner) SetHangupHandler(handler func(cs *CallSession)) {
	r.onHangup = handler
}

// Start begins audio forwarding and STT event processing for an active session.
func (r *MediaRunner) Start(cs *CallSession) {
	cs.mediaMu.Lock()
	if cs.mediaStarted {
		cs.mediaMu.Unlock()
		return
	}
	cs.mediaStarted = true
	cs.sttFactory = r.sttFactory
	cs.onCommittedTranscript = r.onCommitted
	cs.audioCh = make(chan []byte, audioForwardBufferSize)
	cs.mediaDone = make(chan struct{})
	cs.mediaMu.Unlock()

	go r.runReadLoop(cs)
	go r.runAudioForwarder(cs)
	go r.runSTTEventLoop(cs)
}

func (r *MediaRunner) runReadLoop(cs *CallSession) {
	defer close(cs.mediaDone)

	audiosocket.RunReadLoop(cs.Conn, audiosocket.ReadLoopConfig{
		OnAudio: func(pcm []byte) {
			state := cs.CurrentState()
			if state != StateListening && state != StateProcessing && state != StateSpeaking {
				return
			}
			select {
			case cs.audioCh <- pcm:
			default:
				log.WithFields(cs.logFields()).Debug("telephony: audio forward buffer full, dropping frame")
			}
		},
		OnHangup: func() {
			log.WithFields(cs.logFields()).Info("telephony: caller hangup received")
			if r.onHangup != nil {
				r.onHangup(cs)
				return
			}
			if cs.teardown != nil {
				cs.teardown.Complete(cs, "caller_hangup")
				return
			}
			cs.Teardown("caller_hangup")
		},
	})
}

func (r *MediaRunner) runAudioForwarder(cs *CallSession) {
	for {
		select {
		case pcm, ok := <-cs.audioCh:
			if !ok {
				return
			}
			if err := cs.forwardAudioToSTT(pcm); err != nil {
				log.WithFields(cs.logFields()).WithError(err).Warn("telephony: failed to forward audio to STT")
			}
		case <-cs.mediaDone:
			return
		}
	}
}

func (r *MediaRunner) runSTTEventLoop(cs *CallSession) {
	for {
		sttSession := cs.activeSTT()
		if sttSession == nil {
			select {
			case <-cs.mediaDone:
				return
			case <-time.After(50 * time.Millisecond):
				continue
			}
		}

		select {
		case evt, ok := <-sttSession.Events():
			if !ok {
				return
			}
			switch evt.Kind {
			case stt.EventPartialTranscript:
				cs.handleSTTEvent(evt)
			case stt.EventCommittedTranscript:
				cs.handleSTTEvent(evt)
			case stt.EventClosed:
				if evt.Closed.Err != nil {
					if err := cs.reconnectSTT(context.Background()); err != nil {
						log.WithFields(cs.logFields()).WithError(err).Error("telephony: STT reconnect failed")
					}
				}
			}
		case <-cs.mediaDone:
			return
		}
	}
}

func (cs *CallSession) forwardAudioToSTT(pcm8k []byte) error {
	cs.mediaMu.Lock()
	cs.lastAudioAt = time.Now()
	cs.mediaMu.Unlock()

	pcm16k, err := stt.Upsample8kTo16k(pcm8k)
	if err != nil {
		return err
	}

	sttSession := cs.activeSTT()
	if sttSession == nil {
		return nil
	}
	return sttSession.Send(pcm16k)
}

func (cs *CallSession) activeSTT() stt.STTSession {
	cs.StateMu.RLock()
	defer cs.StateMu.RUnlock()
	return cs.STT
}

func (cs *CallSession) setSTT(session stt.STTSession) {
	cs.StateMu.Lock()
	defer cs.StateMu.Unlock()
	cs.STT = session
}

func (cs *CallSession) reconnectSTT(ctx context.Context) error {
	if cs.sttFactory == nil || cs.VoiceConfig == nil {
		return nil
	}

	old := cs.activeSTT()
	newSession, err := OpenSTTSession(ctx, cs.sttFactory, cs.VoiceConfig)
	if err != nil {
		return err
	}

	cs.StateMu.Lock()
	cs.Language = cs.VoiceConfig.Language
	cs.StateMu.Unlock()

	cs.setSTT(newSession)
	if old != nil {
		_ = old.Close()
	}

	log.WithFields(cs.logFields()).Info("telephony: STT session reconnected")
	return nil
}

// handleSTTEvent processes partial and committed transcript events from STT.
func (cs *CallSession) handleSTTEvent(evt stt.Event) {
	switch evt.Kind {
	case stt.EventPartialTranscript:
		cs.mediaMu.Lock()
		cs.partialText = evt.PartialTranscript.Text
		cs.lastHandedOffText = ""
		cs.mediaMu.Unlock()
		cs.ensureBargeIn()
		cs.BargeIn.Trigger()
	case stt.EventCommittedTranscript:
		cs.handleCommittedTranscript(evt.CommittedTranscript.Text)
	}
}

func (cs *CallSession) handleCommittedTranscript(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	cs.mediaMu.Lock()
	defer cs.mediaMu.Unlock()

	if text == cs.lastHandedOffText && cs.lastHandedOffCommitSeq > 0 {
		return
	}

	cs.lastHandedOffCommitSeq++
	cs.lastHandedOffText = text

	turn := &Turn{
		CommittedText: text,
		StartedAt:     time.Now(),
	}
	cs.CurrentTurn = turn
	cs.partialText = ""

	if cs.metrics != nil {
		lastAudio := cs.lastAudioAt
		if !lastAudio.IsZero() {
			cs.metrics.ObserveSTTCommitLatency(time.Since(lastAudio).Seconds())
		}
	}

	if cs.onCommittedTranscript != nil {
		cs.onCommittedTranscript(cs, turn)
	}
}

// PartialText returns the latest partial transcript text (for tests and barge-in wiring).
func (cs *CallSession) PartialText() string {
	cs.mediaMu.Lock()
	defer cs.mediaMu.Unlock()
	return cs.partialText
}

// HandoffCount returns how many committed transcripts were handed off (for tests).
func (cs *CallSession) HandoffCount() int {
	cs.mediaMu.Lock()
	defer cs.mediaMu.Unlock()
	return cs.lastHandedOffCommitSeq
}
