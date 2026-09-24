package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ilhasoft/wwcs/config"
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
		return
	}

	if r.metrics != nil {
		r.metrics.ObserveCallSetupDuration(time.Since(started).Seconds())
	}
}

func (r *SetupRunner) setup(ctx context.Context, cs *CallSession) error {
	log.WithFields(cs.logFields()).WithField("step", "setup_begin").Info("telephony: call setup started")
	cs.ensureAudioKeepalive()

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

	log.WithFields(cs.logFields()).WithField("step", "voice_config_loaded").WithFields(voiceConfigLogFields(cs.VoiceConfig)).Info("telephony: voice config loaded for setup")

	if cs.VoiceConfig.ElevenLabsAPIKey == "" {
		log.WithFields(cs.logFields()).WithField("step", "stt_open").Warn("telephony: setup failed, ElevenLabs API key is empty")
		return &VoiceError{
			Code:        ErrSTTUnavailable,
			Message:     "ElevenLabs API key not configured",
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}

	log.WithFields(cs.logFields()).WithFields(log.Fields{
		"step":         "stt_open",
		"stt_model_id": cs.VoiceConfig.STTModelID,
		"language":     cs.VoiceConfig.Language,
	}).Info("telephony: opening ElevenLabs STT session")

	cs.ttsFactory = r.ttsFactory
	cs.metrics = r.metrics
	cs.Language = cs.VoiceConfig.Language

	greeting := ResolveGreetingText(cs.Language)
	if err := r.playGreeting(ctx, cs, greeting); err != nil {
		return &VoiceError{
			Code:        ErrMediaError,
			Message:     err.Error(),
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}

	sttSession, err := OpenSTTSession(ctx, r.sttFactory, cs.VoiceConfig)
	if err != nil {
		log.WithFields(cs.logFields()).WithField("step", "stt_open").WithError(err).Error("telephony: failed to open ElevenLabs STT session")
		return &VoiceError{
			Code:        ErrSTTUnavailable,
			Message:     err.Error(),
			SpokenKey:   "voice.error.stt_unavailable",
			Recoverable: false,
		}
	}
	cs.STT = sttSession

	log.WithFields(cs.logFields()).WithField("step", "greeting_playback").Info("telephony: greeting played")

	if err := cs.transition(StateListening); err != nil {
		return err
	}

	if r.mediaRunner != nil {
		r.mediaRunner.Start(cs)
	}

	log.WithFields(cs.logFields()).WithField("language", cs.Language).Info("telephony session ready")
	return nil
}

func (r *SetupRunner) handleSetupFailure(cs *CallSession, err error) {
	voiceErr := asVoiceError(err)
	log.WithFields(cs.logFields()).WithFields(log.Fields{
		"step":        "setup_failed",
		"error_code":  voiceErr.Code,
		"error_msg":   voiceErr.Message,
		"spoken_key":  voiceErr.SpokenKey,
		"recoverable": voiceErr.Recoverable,
	}).WithError(err).Error("telephony: call setup failed")

	_ = cs.transition(StateError)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cs.ensureAudioKeepalive()

	if spoken := ResolveSpokenText(voiceErr.SpokenKey, cs.Language); spoken != "" {
		if playErr := r.playSpokenText(ctx, cs, spoken); playErr != nil {
			log.WithFields(cs.logFields()).WithError(playErr).Warn("failed to play spoken fallback")
		}
	}

	r.ensureTeardownCoordinator(cs)
	if cs.teardown != nil {
		cs.teardown.Complete(cs, string(voiceErr.Code))
		return
	}
	cs.Teardown(string(voiceErr.Code))
	if r.onRemove != nil {
		r.onRemove(cs.ID)
	}
}

func (r *SetupRunner) ensureTeardownCoordinator(cs *CallSession) {
	if cs.teardown != nil {
		return
	}
	cs.teardown = &TeardownCoordinator{
		DeliveryCoordinator: r.deliveryCoordinator,
		Metrics:             r.metrics,
		onRemove:            r.onRemove,
	}
}

func (r *SetupRunner) playGreeting(ctx context.Context, cs *CallSession, text string) error {
	audioPath := strings.TrimSpace(config.Get().Telephony.GreetingAudioPath)
	if audioPath != "" {
		pcm, source, err := loadGreetingPCM(audioPath)
		if err != nil {
			return fmt.Errorf("load greeting audio: %w", err)
		}
		log.WithFields(cs.logFields()).WithFields(log.Fields{
			"step":      "greeting_playback",
			"source":    source,
			"pcm_bytes": len(pcm),
		}).Info("telephony: playing non-TTS greeting audio")
		return r.playPCM(ctx, cs, pcm)
	}
	return r.playSpokenText(ctx, cs, text)
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

	var pcm []byte
	for chunk := range audioCh {
		pcm = append(pcm, chunk...)
	}
	if len(pcm) == 0 {
		log.WithFields(cs.logFields()).WithField("step", "tts_playback").Warn("telephony: TTS returned no audio data")
		return nil
	}
	return r.playPCM(ctx, cs, pcm)
}

func (r *SetupRunner) playPCM(ctx context.Context, cs *CallSession, pcm []byte) error {
	if cs.Conn == nil {
		return fmt.Errorf("no audiosocket connection")
	}
	if len(pcm) == 0 {
		log.WithFields(cs.logFields()).WithField("step", "tts_playback").Warn("telephony: no PCM data to play")
		return nil
	}

	cs.pauseAudioKeepalive()
	n, err := writeAudioFrames(cs.Conn, pcm)
	cs.resumeAudioKeepalive()
	if err != nil {
		return err
	}
	if n == 0 {
		log.WithFields(cs.logFields()).WithField("step", "tts_playback").Warn("telephony: PCM playback wrote 0 bytes")
	}
	return nil
}

const audioFrameInterval = 20 * time.Millisecond

func writeAudioFrames(conn audiosocket.AudioSocketConn, pcm []byte) (int, error) {
	written := 0
	for offset := 0; offset < len(pcm); offset += audioFrameSize {
		end := offset + audioFrameSize
		var frame []byte
		if end <= len(pcm) {
			frame = pcm[offset:end]
		} else {
			frame = make([]byte, audioFrameSize)
			copy(frame, pcm[offset:])
		}
		if err := conn.WriteAudio(frame); err != nil {
			return written, err
		}
		written += len(frame)
		time.Sleep(audioFrameInterval)
	}
	return written, nil
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
