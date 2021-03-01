package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/ilhasoft/wwcs/config"
	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		ForceColors:     true,
		FullTimestamp:   true,
		TimestampFormat: "2006/01/02 15:04:05",
	})
}

func main() {
	log.Info("Starting...")

	setupRoutes()

	log.Info("Server is running")
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", config.Get.Port), nil))
}
