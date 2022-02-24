package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/evalphobia/logrus_sentry"
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
	"github.com/ilhasoft/wwcs/pkg/tasks"
	"github.com/ilhasoft/wwcs/pkg/websocket"
	log "github.com/sirupsen/logrus"
)

func init() {
	level, err := log.ParseLevel(config.Get.LogLevel)
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

	if config.Get.SentryDSN != "" {
		hook, err := logrus_sentry.NewSentryHook(config.Get.SentryDSN, []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel})
		hook.Timeout = 0
		hook.StacktraceConfiguration.Enable = true
		hook.StacktraceConfiguration.Skip = 4
		hook.StacktraceConfiguration.Context = 5
		if err != nil {
			log.Fatalf("invalid sentry DSN: '%s': %s", config.Get.SentryDSN, err)
		}
		log.StandardLogger().Hooks.Add(hook)
	}
}

func main() {
	log.Info("Starting...")

	queueConfig := config.Get.RedisQueue
	redisUrl, err := redis.ParseURL(queueConfig.URL)
	if err != nil {
		panic(err)
	}
	rdb := redis.NewClient(redisUrl)
	queueConn := queue.OpenConnection(queueConfig.Tag, rdb, nil)
	defer queueConn.Close()
	qout := queueConn.OpenQueue("outgoing")
	qoutRetry := queueConn.OpenQueue("outgoing-retry")
	qoutRetry.SetPrefetchLimit(queueConfig.ConsumerPrefetchLimit)
	qoutRetry.SetPollDuration(time.Duration(queueConfig.RetryPollDuration) * time.Millisecond)
	qout.SetPushQueue(qoutRetry)

	outQueueConsumer := queue.NewConsumer(qout)
	outRetryQueueConsumer := queue.NewConsumer(qoutRetry)

	metrics, err := metric.NewPrometheusService()
	if err != nil {
		log.Fatal(err)
	}

	app := websocket.NewApp(websocket.NewPool(), qout, rdb, metrics)

	outQueueConsumer.StartConsuming(5, tasks.NewTasks(app).SendMsgToExternalService)
	outRetryQueueConsumer.StartConsuming(5, tasks.NewTasks(app).SendMsgToExternalService)

	websocket.SetupRoutes(app)

	queueConn.NewCleaner()

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
}
