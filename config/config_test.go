package config

import (
	"os"
	"testing"
)

var envCases = map[string]string{
	"WWC_PORT":                          "port",
	"WWC_WEBSOCKET_REDIRECTTOFRONTEND":  "true",
	"WWC_WEBSOCKET_REDIRECTTOCALLBACK":  "true",
	"WWC_WEBSOCKET_SENDWELLCOMEMESSAGE": "true",
	"WWC_WEBSOCKET_WELLCOMEMESSAGE":     "wellcomeMessage",
}

var confTest = Configuration{
	Port: "port",
	Websocket: Websocket{
		RedirectToCallback:  true,
		RedirectToFrontend:  true,
		SendWellcomeMessage: true,
		WellcomeMessage:     "wellcomeMessage",
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
