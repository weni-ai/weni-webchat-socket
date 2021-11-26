package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/pkg/queue"
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

	qconn := queue.NewQueueConnection("wwcs-service")
	qout := queue.OpenQueue("wwcs-outgoing", qconn)
	qinc := queue.OpenQueue("wwcs-incoming", qconn)
	qoutProducer := queue.NewProducer(qout)
	qincProducer := queue.NewProducer(qinc)

	app := websocket.NewApp(websocket.NewPool(), qoutProducer, qincProducer)

	websocket.SetupRoutes(app)

	taskSendMsgToContact := func(payload string) error {
		log.Print(payload)
		return nil
	}
	taskSendMsgToRapidPro := func(payload string) error {
		log.Print(payload)
		var sJob websocket.OutgoingJob
		if err := json.Unmarshal([]byte(payload), &sJob); err != nil {
			return err
		}
		err := websocket.ToCallback(sJob.URL, sJob.Payload)
		if err != nil {
			return err
		}
		return nil
	}

	queue.StartConsuming(qinc, taskSendMsgToContact)
	queue.StartConsuming(qout, taskSendMsgToRapidPro)

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
}
