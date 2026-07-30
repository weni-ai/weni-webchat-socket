package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/db"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/jwt"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/telephony/audiosocket"
	"github.com/ilhasoft/wwcs/pkg/telephony/session"
	"github.com/ilhasoft/wwcs/pkg/telephony/stt"
	"github.com/ilhasoft/wwcs/pkg/telephony/tts"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func init() {
	level, err := log.ParseLevel(os.Getenv("WWC_LOG_LEVEL"))
	if err != nil {
		level = log.InfoLevel
		log.WithError(err).WithField("level", level).Error("unable to set log level")
	}
	log.SetOutput(os.Stdout)
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     false,
		FullTimestamp:   true,
		TimestampFormat: "2006/01/02 15:04:05",
	})
	log.SetReportCaller(true)

	sentryDSN := os.Getenv("WWC_APP_SENTRY_DSN")
	if sentryDSN != "" {
		hook, err := logrus_sentry.NewSentryHook(config.Get().SentryDSN, []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel})
		hook.Timeout = 0
		hook.StacktraceConfiguration.Enable = true
		hook.StacktraceConfiguration.Skip = 4
		hook.StacktraceConfiguration.Context = 5
		if err != nil {
			log.Fatalf("invalid sentry DSN: '%s': %s", config.Get().SentryDSN, err)
		}
		log.StandardLogger().Hooks.Add(hook)
	}
}

func main() {
	var httpPort string
	flag.StringVar(&httpPort, "p", "", "telephony HTTP registration port")
	flag.Parse()

	log.Info("Starting telephony voice gateway...")

	cfg := config.Get()
	telephonyCfg := cfg.Telephony

	queueConfig := cfg.RedisQueue
	rdbClientOptions, err := redis.ParseURL(queueConfig.URL)
	if err != nil {
		panic(err)
	}
	rdbClientOptions.MaxRetries = int(queueConfig.MaxRetries)
	redisTimeout := time.Second * time.Duration(queueConfig.Timeout)
	rdb := redis.NewClient(rdbClientOptions).WithTimeout(redisTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal(errors.Wrap(err, "unable to connect to redis"))
	}

	mdb := db.NewDB()

	var jwtSigner *jwt.Signer
	jwtConfig := cfg.JWT
	if jwtConfig.PrivateKey != "" {
		jwtSigner, err = jwt.NewSigner(jwtConfig.PrivateKey, jwtConfig.ExpirationMins)
		if err != nil {
			log.Fatal(errors.Wrap(err, "failed to initialize JWT signer"))
		}
		log.Info("JWT signer initialized successfully")
	} else {
		log.Warn("JWT private key not configured, Flows API calls will not be authenticated")
	}

	flowsClient := flows.NewClient(cfg.FlowsURL, jwtSigner)

	baseMetrics, err := metric.NewPrometheusService()
	if err != nil {
		log.Fatal(errors.Wrap(err, "failed to initialize metrics"))
	}

	sessionMetrics, err := session.NewSessionMetrics(baseMetrics)
	if err != nil {
		log.Fatal(errors.Wrap(err, "failed to initialize telephony metrics"))
	}

	sttClient := stt.NewClient(telephonyCfg.ElevenLabsAPIURL, nil)
	sttFactory := func(ctx context.Context, cfg *session.VoiceConfig) (stt.STTSession, error) {
		return sttClient.OpenSession(ctx, stt.SessionConfig{
			APIKey:       cfg.ElevenLabsAPIKey,
			ModelID:      cfg.STTModelID,
			Language:     cfg.Language,
			VADSilenceMs: cfg.VADSilenceMs,
		})
	}
	ttsFactory := func(cfg *session.VoiceConfig) tts.TTSStreamClient {
		return tts.NewClient(telephonyCfg.ElevenLabsAPIURL, cfg.ElevenLabsAPIKey, cfg.TTSModelID, nil)
	}

	podID := fmt.Sprintf("telephony-%s", os.Getenv("HOSTNAME"))
	if podID == "telephony-" {
		podID = fmt.Sprintf("telephony-%d", time.Now().Unix())
	}

	clientManager := websocket.NewClientManager(rdb, int(queueConfig.ClientTTL))

	streamsCfg := streams.StreamsConfig{
		StreamsMaxLenApprox: queueConfig.StreamsMaxLen,
		StreamsReadCount:    queueConfig.StreamsReadCount,
		StreamsBlockMs:      queueConfig.StreamsBlockMs,
		StreamsClaimIdleMs:  queueConfig.StreamsClaimIdleMs,
		HeartbeatTTLSeconds: queueConfig.ClientTTL,
		JanitorIntervalMs:   queueConfig.JanitorIntervalMs,
		JanitorLeaseMs:      queueConfig.JanitorLeaseMs,
		StreamsRetentionMs:  queueConfig.StreamsRetentionMs,
		StreamsMaxPendingMs: queueConfig.StreamsMaxPendingAgMs,
		DeadPodRetentionMs:  queueConfig.DeadPodRetentionMs,
	}

	sessionManager := session.NewSessionManager(
		flowsClient,
		telephonyCfg.MaxConcurrentCalls,
		telephonyCfg.HoldAudioPath,
		sessionMetrics,
		nil,
	)

	streamsRouter := session.NewTelephonyStreamsRouter(rdb, streamsCfg, podID, clientManager, sessionManager)
	routerCtx, routerCancel := context.WithCancel(context.Background())
	defer routerCancel()
	go streamsRouter.Start(routerCtx)

	deliveryCoordinator := session.NewDeliveryCoordinator(clientManager, sessionManager, podID)
	mediaRunner := session.NewMediaRunner(sttFactory, deliveryCoordinator.OnCommittedTranscript)
	setupRunner := session.NewSetupRunner(flowsClient, sttFactory, ttsFactory, sessionMetrics, mediaRunner, deliveryCoordinator, nil)
	sessionManager.SetSetupRunner(setupRunner)

	if httpPort == "" {
		httpPort = telephonyCfg.HTTPPort
	}

	advertiseHost := os.Getenv("WWC_TELEPHONY_ADVERTISE_HOST")
	if advertiseHost == "" {
		advertiseHost = "localhost"
	}
	audiosocketAddr := fmt.Sprintf("%s:%s", advertiseHost, telephonyCfg.AudioSocketPort)

	audioServer := audiosocket.NewServer(
		fmt.Sprintf(":%s", telephonyCfg.AudioSocketPort),
		func(sessionID string, conn audiosocket.AudioSocketConn) {
			if err := sessionManager.Attach(sessionID, conn); err != nil {
				log.WithFields(log.Fields{
					"session_id": sessionID,
				}).WithError(err).Warn("failed to attach audiosocket session")
				_ = conn.Close()
			}
		},
	)

	if err := audioServer.Start(); err != nil {
		log.Fatal(errors.Wrap(err, "failed to start audiosocket server"))
	}

	mux := http.NewServeMux()
	mux.Handle("/telephony/sessions", &audiosocket.RegistrationHandler{
		Registrar:       sessionManager,
		AudioSocketAddr: audiosocketAddr,
	})

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", httpPort),
		Handler: mux,
	}

	go func() {
		log.WithField("port", httpPort).Info("telephony registration HTTP server listening")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(errors.Wrap(err, "telephony HTTP server failed"))
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("received signal %v, shutting down", sig)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Warn("telephony HTTP server shutdown error")
	}
	if err := audioServer.Stop(); err != nil {
		log.WithError(err).Warn("audiosocket server shutdown error")
	}
	streamsRouter.Stop(context.Background())

	_ = mdb
	_ = rdb
}
