package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/adjust/rmq/v4"
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/config"
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
}

func main() {
	log.Info("Starting...")
	queueConfig := config.Get.RedisQueue

	rdb := redis.NewClient(&redis.Options{Addr: queueConfig.Address, DB: queueConfig.DB})
	rmqConnection, err := rmq.OpenConnectionWithRedisClient(queueConfig.Tag, rdb, nil)
	if err != nil {
		panic(err)
	}
	qout := queue.OpenQueue("outgoing", rmqConnection)

	outQueueConsumer := queue.NewConsumer(qout)
	app := websocket.NewApp(websocket.NewPool(), qout, rdb)
	outQueueConsumer.StartConsuming(tasks.NewTasks(app).SendMsgToExternalService)

	websocket.SetupRoutes(app)

	queue.NewCleaner(rmqConnection)

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
	rmqConnection.StopAllConsuming()
}
