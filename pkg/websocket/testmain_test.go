package websocket

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// Set required environment variables for config loading
	requiredEnvVars := map[string]string{
		"WWC_S3_ACCESS_KEY":   "test-access-key",
		"WWC_S3_SECRET_KEY":   "test-secret-key",
		"WWC_S3_ENDPOINT":     "http://localhost:9000",
		"WWC_S3_REGION":       "us-east-1",
		"WWC_S3_BUCKET":       "test-bucket",
		"WWC_JWT_PRIVATE_KEY": "test-private-key",
	}

	for k, v := range requiredEnvVars {
		os.Setenv(k, v)
	}

	code := m.Run()

	// Cleanup environment variables
	for k := range requiredEnvVars {
		os.Unsetenv(k)
	}

	os.Exit(code)
}
