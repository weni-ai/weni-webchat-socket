package session

import (
	"context"
	"fmt"
	"time"

	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	log "github.com/sirupsen/logrus"
)

const audioFrameSize = 320

// STTSessionFactory opens a gateway-side STT session for a call.
type STTSessionFactory func(ctx context.Context, cfg *VoiceConfig) (stt.STTSession, error)

// TTSClientFactory returns a TTS client configured for the call's tenant.
type TTSClientFactory func(cfg *VoiceConfig) tts.TTSStreamClient

// SetupRunner orchestrates call setup, greeting playback, and error teardown.
type SetupRunner struct {
	flowsClient         flows.IClient
	sttFactory          STTSessionFactory
	ttsFactory          TTSClientFactory
	metrics             *SessionMetrics
	mediaRunner         *MediaRunner
	deliveryCoordinator *DeliveryCoordinator
	onRemove            func(sessionID string)
}

// NewSetupRunner creates a SetupRunner with the given dependencies.
func NewSetupRunner(
	flowsClient flows.IClient,
	sttFactory STTSessionFactory,
	ttsFactory TTSClientFactory,
	metrics *SessionMetrics,
	mediaRunner *MediaRunner,
	deliveryCoordinator *DeliveryCoordinator,
	onRemove func(sessionID string),
) *SetupRunner {
	return &SetupRunner{
		flowsClient:         flowsClient,
		sttFactory:          sttFactory,
		ttsFactory:          ttsFactory,
		metrics:             metrics,
		mediaRunner:         mediaRunner,
		deliveryCoordinator: deliveryCoordinator,
		onRemove:            onRemove,
	}
}

// Run executes setup asynchronously for an attached session.
func (r *SetupRunner) Run(cs *CallSession) {
	go r.run(cs)
}

func (r *SetupRunner) run(cs *CallSession) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := r.setup(ctx, cs); err != nil {
		r.handleSetupFailure(cs, err)
		if r.metrics != nil {
			r.metrics.IncCallTeardown(reasonFromError(err))
		}
		return
	}

	if r.metrics != nil {
		r.metrics.ObserveCallSetupDuration(time.Since(started).Seconds())
	}
}

func (r *SetupRunner) setup(ctx context.Context, cs *CallSession) error {
	if cs.VoiceConfig == nil {
		cfg, err := ResolveVoiceConfig(r.flowsClient, cs.ChannelUUID)
		if err != nil {
			return &VoiceError{
				Code:        ErrChannelUnresolved,
				Message:     err.Error(),
				SpokenKey:   "voice.error.channel_unresolved",
				Recoverable: false,
			}
		}
		cs.VoiceConfig = cfg
		cs.Language = cfg.Language
	}

	if cs.VoiceConfig.ElevenLabsAPIKey == "" {
		return &VoiceError{
			Code:        ErrSTTUnavailable,
			Message:     "ElevenLabs API key not configured",
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}

	sttSession, err := r.sttFactory(ctx, cs.VoiceConfig)
	if err != nil {
		return &VoiceError{
			Code:        ErrSTTUnavailable,
			Message:     err.Error(),
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}
	cs.STT = sttSession

	greeting := ResolveGreetingText(cs.Language)
	if err := r.playSpokenText(ctx, cs, greeting); err != nil {
		return &VoiceError{
			Code:        ErrMediaError,
			Message:     err.Error(),
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}

	if err := cs.transition(StateListening); err != nil {
		return err
	}

	if r.mediaRunner != nil {
		r.mediaRunner.Start(cs)
	}

	log.WithFields(log.Fields{
		"session_id":   cs.ID,
		"channel_uuid": cs.ChannelUUID,
		"language":     cs.Language,
	}).Info("telephony session ready")
	return nil
}

func (r *SetupRunner) handleSetupFailure(cs *CallSession, err error) {
	voiceErr := asVoiceError(err)
	_ = cs.transition(StateError)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if spoken := ResolveSpokenText(voiceErr.SpokenKey, cs.Language); spoken != "" {
		if playErr := r.playSpokenText(ctx, cs, spoken); playErr != nil {
			log.WithFields(log.Fields{
				"session_id": cs.ID,
			}).WithError(playErr).Warn("failed to play spoken fallback")
		}
	}

	r.teardown(cs, voiceErr.Code)
}

func (r *SetupRunner) teardown(cs *CallSession, reason ErrorCode) {
	if r.deliveryCoordinator != nil {
		r.deliveryCoordinator.TeardownDelivery(cs)
	}
	if cs.STT != nil {
		_ = cs.STT.Close()
		cs.STT = nil
	}
	if cs.Conn != nil {
		_ = cs.Conn.Close()
		cs.Conn = nil
	}
	_ = cs.transition(StateEnded)
	if r.onRemove != nil {
		r.onRemove(cs.ID)
	}

	log.WithFields(log.Fields{
		"session_id": cs.ID,
		"reason":     reason,
	}).Info("telephony session torn down")
}

func (r *SetupRunner) playSpokenText(ctx context.Context, cs *CallSession, text string) error {
	if cs.Conn == nil {
		return fmt.Errorf("no audiosocket connection")
	}
	if cs.VoiceConfig == nil {
		return fmt.Errorf("voice config not resolved")
	}
	if r.ttsFactory == nil {
		return fmt.Errorf("tts factory not configured")
	}

	ttsClient := r.ttsFactory(cs.VoiceConfig)
	audioCh, err := ttsClient.Synthesize(ctx, text, cs.VoiceConfig.VoiceID, cs.Language)
	if err != nil {
		return err
	}

	for chunk := range audioCh {
		if err := writeAudioFrames(cs.Conn, chunk); err != nil {
			return err
		}
	}
	return nil
}

func writeAudioFrames(conn audiosocket.AudioSocketConn, pcm []byte) error {
	for offset := 0; offset < len(pcm); offset += audioFrameSize {
		end := offset + audioFrameSize
		if end > len(pcm) {
			end = len(pcm)
		}
		if err := conn.WriteAudio(pcm[offset:end]); err != nil {
			return err
		}
	}
	return nil
}

func asVoiceError(err error) *VoiceError {
	if err != nil {
		if ve, ok := err.(*VoiceError); ok {
			return ve
		}
		return &VoiceError{
			Code:        ErrMediaError,
			Message:     err.Error(),
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}
	return &VoiceError{
		Code:        ErrMediaError,
		Message:     "unknown error",
		SpokenKey:   "voice.error.stt_unavailable",
		Recoverable: false,
	}
}

func reasonFromError(err error) string {
	voiceErr := asVoiceError(err)
	if voiceErr.Code != "" {
		return string(voiceErr.Code)
	}
	return "unknown"
}
