package config

import (
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

// Get all configs from env vars or config file
var Get = loadConfigs()

// Configuration struct
type Configuration struct {
	Port      string `default:"8080" env:"WWC_PORT"`
	LogLevel  string `default:"info" env:"WWC_LOG_LEVEL"`
	Websocket Websocket
}

// Websocket struct
type Websocket struct {
	RedirectToCallback bool `default:"true"  env:"WWC_WEBSOCKET_REDIRECT_TO_CALLBACK"`
	RedirectToFrontend bool `default:"false" env:"WWC_WEBSOCKET_REDIRECT_TO_FRONTEND"`
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
