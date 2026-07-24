package main

import (
	"context"
	"flag"
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
	_ = flowsClient

	if httpPort == "" {
		httpPort = telephonyCfg.HTTPPort
	}

	// TODO(Phase 2, T017): wire SessionManager, AudioSocket TCP server, and
	// POST /telephony/sessions registration HTTP handler here.

	log.WithFields(log.Fields{
		"http_port":          httpPort,
		"audiosocket_port":   telephonyCfg.AudioSocketPort,
		"max_concurrent":     telephonyCfg.MaxConcurrentCalls,
		"flows_url":          cfg.FlowsURL,
		"redis_connected":    true,
		"mongodb_connected":  mdb != nil,
	}).Info("telephony bootstrap complete; SessionManager wiring pending Phase 2")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Infof("received signal %v, shutting down", sig)
}
