package config

import (
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

// Get all configs from env vars or config file
var Get = loadConfigs()

// Configuration struct
type Configuration struct {
	Port      string `default:"8080"`
	Websocket Websocket
}

// Websocket struct
type Websocket struct {
	RedirectToCallback  bool   `default:"true"`
	RedirectToFrontend  bool   `default:"false"`
	SendWellcomeMessage bool   `default:"true"`
	WellcomeMessage     string `default:"Wellcome to weni webchat!"`
}

func loadConfigs() (config Configuration) {
	log.Trace("Loading configs")
	settings := configor.Config{
		ENVPrefix: "WWC",
		Silent:    true,
	}

	if err := configor.New(&settings).Load(&config, "config.json"); err != nil {
		log.Fatal(err)
	}

	return config
}
