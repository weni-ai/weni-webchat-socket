package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/ilhasoft/wwcs/config"
	"github.com/ilhasoft/wwcs/handler"
	log "github.com/sirupsen/logrus"
)

func init() {
	level, err := log.ParseLevel(config.Get.LogLevel)
	if err != nil {
		level = log.InfoLevel
		log.Error(`unable to set log level: %v: level %s was setted`, err, level)
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

	handler.SetupRoutes()

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
}
