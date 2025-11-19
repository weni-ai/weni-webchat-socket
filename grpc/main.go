package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/db"
	grpcserver "github.com/ilhasoft/wwcs/pkg/grpc"
	grpcHealth "github.com/ilhasoft/wwcs/pkg/grpc/health"
	"github.com/ilhasoft/wwcs/pkg/grpc/proto"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func init() {
	level, err := log.ParseLevel(os.Getenv("WWC_LOG_LEVEL"))
	if err != nil {
		level = log.InfoLevel
		log.Errorf(`unable to set log level: %v: level %s was set`, err, level)
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
	var grpcAddr string
	flag.StringVar(&grpcAddr, "grpc-addr", "", "gRPC server address (e.g., :50051)")
	flag.Parse()

	log.Info("Starting gRPC Message Stream Server...")

	// Get gRPC address from flag or environment
	if grpcAddr == "" {
		grpcAddr = os.Getenv("GRPC_SERVER_ADDR")
		if grpcAddr == "" {
			grpcAddr = ":50051" // Default
		}
	}

	// Initialize Redis connection (shared with WebSocket pods)
	queueConfig := config.Get().RedisQueue
	rdbClientOptions, err := redis.ParseURL(queueConfig.URL)
	if err != nil {
		log.Fatal(errors.Wrap(err, "failed to parse Redis URL"))
	}
	rdbClientOptions.MaxRetries = int(config.Get().RedisQueue.MaxRetries)
	redisTimeout := time.Second * time.Duration(config.Get().RedisQueue.Timeout)
	rdb := redis.NewClient(rdbClientOptions).WithTimeout(redisTimeout)

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()
	rdbPing := rdb.Ping(ctx)
	if rdbPing.Err() != nil {
		log.Fatal(errors.Wrap(rdbPing.Err(), "unable to connect to Redis"))
	}
	log.Info("Connected to Redis")

	// Initialize MongoDB for history
	mdb := db.NewDB()
	histories := history.NewService(history.NewRepo(mdb, config.Get().DB.ContextTimeout))
	log.Info("Connected to MongoDB")

	// Initialize client manager (to check if clients are connected)
	clientM := websocket.NewClientManager(rdb, int(queueConfig.ClientTTL))

	// Create a minimal Router for gRPC server (doesn't need full pool)
	// The gRPC server only publishes messages, doesn't consume them
	podID := fmt.Sprintf("grpc-server-%s", os.Getenv("HOSTNAME"))
	if podID == "grpc-server-" {
		podID = fmt.Sprintf("grpc-server-%d", time.Now().Unix())
	}

	streamsCfg := streams.StreamsConfig{
		StreamsMaxLenApprox: config.Get().RedisQueue.StreamsMaxLen,
		StreamsReadCount:    config.Get().RedisQueue.StreamsReadCount,
		StreamsBlockMs:      config.Get().RedisQueue.StreamsBlockMs,
		StreamsClaimIdleMs:  config.Get().RedisQueue.StreamsClaimIdleMs,
		HeartbeatTTLSeconds: config.Get().RedisQueue.ClientTTL,
		JanitorIntervalMs:   config.Get().RedisQueue.JanitorIntervalMs,
		JanitorLeaseMs:      config.Get().RedisQueue.JanitorLeaseMs,
	}

	// Create a lightweight router for publishing only
	// We need to provide lookup, isLocal, and deliver functions
	// For gRPC server, we only publish, so deliver can be a no-op
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
		// gRPC server doesn't have local clients
		return false
	}

	deliver := func(clientID string, raw []byte) error {
		// gRPC server doesn't deliver locally, only publishes to Redis streams
		return nil
	}

	router := streams.NewRouter(rdb, podID, streamsCfg, lookup, isLocal, deliver)

	// Create app-like structure for gRPC server
	// We don't need full websocket.App, just the interfaces needed by gRPC server
	app := &websocket.App{
		ClientPool:    nil, // gRPC server doesn't have client pool
		RDB:           rdb,
		MDB:           mdb,
		Metrics:       nil, // Could add metrics if needed
		Histories:     histories,
		ClientManager: clientM,
		Router:        router,
		PodID:         podID,
		FlowsClient:   nil, // Not needed for gRPC server
	}

	// Start router heartbeat (so other pods know this gRPC server is alive)
	routerCtx, routerCancel := context.WithCancel(context.Background())
	defer routerCancel()
	go router.Start(routerCtx)

	// Create gRPC server
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", grpcAddr, err)
	}

	grpcServer := grpc.NewServer()
	streamApp := grpcserver.NewMessageStreamApp(app)
	srv := grpcserver.NewServer(streamApp)

	proto.RegisterMessageStreamServiceServer(grpcServer, srv)
	// Start standard gRPC health server + background dependency monitor
	healthMonitor := grpcHealth.StartMonitor(grpcServer, rdb, mdb, "message_stream.MessageStreamService")

	// Enable gRPC reflection for debugging with grpcurl
	reflection.Register(grpcServer)

	log.WithField("address", grpcAddr).Info("gRPC MessageStream server is listening")

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Infof("Received signal %v, shutting down gRPC server", sig)

		// Stop router
		router.Stop(context.Background())
		routerCancel()

		// Graceful stop of gRPC server
		// Stop health monitor (marks NOT_SERVING and stops goroutine)
		healthMonitor.Stop()
		grpcServer.GracefulStop()

		// Remove heartbeat
		hbKey := "ws:pod:hb:" + podID
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := rdb.Del(ctx, hbKey).Err(); err != nil {
			log.WithError(err).Warn("Failed to delete heartbeat key on shutdown")
		}
		cancel()
	}()

	// Start serving
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server failed: %v", err)
	}
}
