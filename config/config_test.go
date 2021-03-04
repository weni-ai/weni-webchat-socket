package config

import (
	"os"
	"testing"
)

var envCases = map[string]string{
	"WWC_PORT":                           "port",
	"WWC_LOG_LEVEL":                      "trace",
	"WWC_WEBSOCKET_REDIRECT_TO_CALLBACK": "true",
	"WWC_WEBSOCKET_REDIRECT_TO_FRONTEND": "true",
}

var confTest = Configuration{
	Port:     "port",
	LogLevel: "trace",
	Websocket: Websocket{
		RedirectToCallback: true,
		RedirectToFrontend: true,
	},
}

func TestLoadConfigs(t *testing.T) {
	for k, v := range envCases {
		os.Setenv(k, v)
	}

	have := loadConfigs()

	if have != confTest {
		t.Errorf("have %#v, want %#v", have, confTest)
	}
}
