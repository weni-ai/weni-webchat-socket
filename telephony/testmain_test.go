package main

import (
	"os"
	"testing"

	"github.com/ilhasoft/wwcs/config"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("WWC_S3_ACCESS_KEY", "test")
	_ = os.Setenv("WWC_S3_SECRET_KEY", "test")
	_ = os.Setenv("WWC_S3_ENDPOINT", "http://localhost")
	_ = os.Setenv("WWC_S3_REGION", "us-east-1")
	_ = os.Setenv("WWC_S3_BUCKET", "test")
	config.Clear()
	os.Exit(m.Run())
}
