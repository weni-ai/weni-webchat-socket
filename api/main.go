package main

import (
	"fmt"
	"net/http"
	"os"

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

	qconn := queue.NewQueueConnection(queueConfig.Tag, queueConfig.Address, queueConfig.DB)
	qout := queue.OpenQueue("wwcs-outgoing", qconn)
	qinc := queue.OpenQueue("wwcs-incoming", qconn)
	qoutProducer := queue.NewProducer(qout)
	qincProducer := queue.NewProducer(qinc)
	qoutConsumer := queue.NewConsumer(qout)
	qincConsumer := queue.NewConsumer(qinc)

	app := websocket.NewApp(websocket.NewPool(), qoutProducer, qincProducer)

	ts := tasks.NewTasks(app)
	websocket.SetupRoutes(app)

	qincConsumer.StartConsuming(ts.SendMsgToContact)
	qoutConsumer.StartConsuming(ts.SendMsgToCourier)

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
	<-qconn.StopAllConsuming()
}
