package config

import (
	"os"
	"testing"
)

var ttDefaultConfigs = Configuration{
	Port:     "8080",
	LogLevel: "info",
	S3: S3{
		Endpoint:       "required",
		Region:         "required",
		Bucket:         "required",
		DisableSSL:     false,
		ForcePathStyle: false,
	},
}

var ttEnvConfigs = Configuration{
	Port:     "1234",
	LogLevel: "trace",
	S3: S3{
		Endpoint:       "endpoint",
		Region:         "region",
		Bucket:         "bucket",
		DisableSSL:     false,
		ForcePathStyle: false,
	},
}

var requiredEnvCases = map[string]string{
	"WWC_S3_ENDPOINT": "required",
	"WWC_S3_REGION":   "required",
	"WWC_S3_BUCKET":   "required",
}

var envCases = map[string]string{
	"WWC_PORT":        "1234",
	"WWC_LOG_LEVEL":   "trace",
	"WWC_S3_ENDPOINT": "endpoint",
	"WWC_S3_REGION":   "region",
	"WWC_S3_BUCKET":   "bucket",
}

func TestLoadConfigs(t *testing.T) {
	for k, v := range requiredEnvCases {
		os.Setenv(k, v)
	}
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
