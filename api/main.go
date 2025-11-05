package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/db"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

func init() {
	level, err := log.ParseLevel(os.Getenv("WWC_LOG_LEVEL"))
	if err != nil {
		level = log.InfoLevel
		log.Errorf(`unable to set log level: %v: level %s was setted`, err, level)
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
	var port string
	flag.StringVar(&port, "p", "", "listening port")
	flag.Parse()

	log.Info("Starting...")

	queueConfig := config.Get().RedisQueue
	rdbClientOptions, err := redis.ParseURL(queueConfig.URL)
	if err != nil {
		panic(err)
	}
	rdbClientOptions.MaxRetries = int(config.Get().RedisQueue.MaxRetries)
	redisTimeout := time.Second * time.Duration(config.Get().RedisQueue.Timeout)
	rdb := redis.NewClient(rdbClientOptions).WithTimeout(redisTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	rdbPing := rdb.Ping(ctx)
	if rdbPing.Err() != nil {
		log.Fatal(errors.Wrap(rdbPing.Err(), "Unable to connect to redis"))
	}

	metrics, err := metric.NewPrometheusService()
	if err != nil {
		log.Fatal(err)
	}

	mdb := db.NewDB()
	histories := history.NewService(history.NewRepo(mdb, config.Get().DB.ContextTimeout))

	clientM := websocket.NewClientManager(rdb, int(queueConfig.ClientTTL))

	flowsClient := flows.NewClient(config.Get().FlowsURL)

	// Derive pod ID
	podID := os.Getenv("POD_NAME")
	if podID == "" {
		podID = os.Getenv("HOSTNAME")
	}
	if podID == "" {
		podID = fmt.Sprintf("pod-%d", time.Now().UnixNano())
	}

	pool := websocket.NewPool()

	// Build Router
	streamsCfg := streams.StreamsConfig{
		StreamsMaxLenApprox: config.Get().RedisQueue.StreamsMaxLen,
		StreamsReadCount:    config.Get().RedisQueue.StreamsReadCount,
		StreamsBlockMs:      config.Get().RedisQueue.StreamsBlockMs,
		StreamsClaimIdleMs:  config.Get().RedisQueue.StreamsClaimIdleMs,
		HeartbeatTTLSeconds: config.Get().RedisQueue.ClientTTL,
		JanitorIntervalMs:   config.Get().RedisQueue.JanitorIntervalMs,
		JanitorLeaseMs:      config.Get().RedisQueue.JanitorLeaseMs,
	}

	lookup := func(clientID string) (string, bool, error) {
		cc, err := clientM.GetConnectedClient(clientID)
		if err != nil {
			return "", false, err
		}
		if cc == nil || cc.PodID == "" {
			return "", false, nil
		}
		return cc.PodID, true, nil
	}

	isLocal := func(clientID string) bool {
		_, ok := pool.Find(clientID)
		return ok
	}

	deliver := func(clientID string, raw []byte) error {
		client, ok := pool.Find(clientID)
		if !ok || client == nil {
			return nil
		}
		var incoming websocket.IncomingPayload
		if err := json.Unmarshal(raw, &incoming); err != nil {
			return err
		}
		if err := client.Send(incoming); err != nil {
			return err
		}
		// Refresh TTL on successful delivery
		_, _ = clientM.UpdateClientTTL(clientID, clientM.DefaultClientTTL())
		return nil
	}

	router := streams.NewRouter(rdb, podID, streamsCfg, lookup, isLocal, deliver)

	app := websocket.NewApp(
		pool,
		rdb,
		mdb,
		metrics,
		histories,
		clientM,
		router,
		podID,
		flowsClient,
	)
	// app.StartConnectionsHeartbeat()
	websocket.SetupRoutes(app)

	go router.Start(context.Background())

	if port == "" {
		port = config.Get().Port
	}

	// log every 30 seconds info about redis connection pool
	go func() {
		for range time.Tick(30 * time.Second) {
			log.WithFields(log.Fields{
				"hits":        rdb.PoolStats().Hits,
				"misses":      rdb.PoolStats().Misses,
				"timeouts":    rdb.PoolStats().Timeouts,
				"total_conns": rdb.PoolStats().TotalConns,
				"idle_conns":  rdb.PoolStats().IdleConns,
				"stale_conns": rdb.PoolStats().StaleConns,
			}).Info("redis connection pool stats")
		}
	}()

	log.Info("listening on port ", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
}
