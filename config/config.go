package config

import (
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

// Get all configs from env vars or config file
var Get = loadConfigs()

// Configuration struct
type Configuration struct {
	Port       string `default:"8080" env:"WWC_PORT"`
	LogLevel   string `default:"info" env:"WWC_LOG_LEVEL"`
	S3         S3
	RedisQueue RedisQueue
}

type S3 struct {
	AccessKey      string `required:"true" env:"WWC_S3_ACCESS_KEY"`
	SecretKey      string `required:"true" env:"WWC_S3_SECRET_KEY"`
	Endpoint       string `required:"true" env:"WWC_S3_ENDPOINT"`
	Region         string `required:"true" env:"WWC_S3_REGION"`
	Bucket         string `required:"true" env:"WWC_S3_BUCKET"`
	DisableSSL     bool   `default:"false" env:"WWC_S3_DISABLE_SSL"`
	ForcePathStyle bool   `default:"false" env:"WWC_S3_FORCE_PATH_STYLE"`
}

type RedisQueue struct {
	Tag                     string `default:"wwcs-service" env:"WWC_REDIS_QUEUE_TAG"`
	Address                 string `default:"localhost:6379" env:"WWC_REDIS_QUEUE_ADDRESS"`
	DB                      int    `default:"1" env:"WWC_REDIS_QUEUE_DB"`
	ConsumerPrefetchLimit   int64  `default:"1000" env:"WWC_REDIS_QUEUE_CONSUMER_PREFETCH_LIMIT"`
	ConsumerPollDuration    int    `default:"100" env:"WWC_REDIS_QUEUE_CONSUMER_POLL_DURATION"`
	ConsumerWorkers         int    `default:"1" env:"WWC_REDIS_QUEUE_CONSUMER_WORKERS"`
	ConsumerConsumeDuration int    `default:"1" env:"WWC_REDIS_QUEUE_CONSUMER_CONSUME_DURATION"`
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
