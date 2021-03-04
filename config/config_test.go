package config

import (
	"os"
	"testing"
)

var ttDefaultConfigs = Configuration{
	Port:     "8080",
	LogLevel: "info",
	Websocket: Websocket{
		RedirectToCallback: true,
		RedirectToFrontend: false,
	},
}

var ttEnvConfigs = Configuration{
	Port:     "1234",
	LogLevel: "trace",
	Websocket: Websocket{
		RedirectToCallback: false,
		RedirectToFrontend: true,
	},
}

var envCases = map[string]string{
	"WWC_PORT":                           "1234",
	"WWC_LOG_LEVEL":                      "trace",
	"WWC_WEBSOCKET_REDIRECT_TO_CALLBACK": "false",
	"WWC_WEBSOCKET_REDIRECT_TO_FRONTEND": "true",
}

func TestLoadConfigs(t *testing.T) {
	t.Run("Default configs", func(t *testing.T) {
		assertConfigs(t, ttDefaultConfigs)
	})

	t.Run("Env configs", func(t *testing.T) {
		for k, v := range envCases {
			os.Setenv(k, v)
			defer os.Unsetenv(k)
		}
		assertConfigs(t, ttEnvConfigs)
	})
}

func assertConfigs(t *testing.T, want Configuration) {
	t.Helper()

	have := loadConfigs()
	if have != want {
		t.Errorf("have %#v, want %#v", have, want)
	}
}
