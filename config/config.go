package config

import (
	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

var configs *Configuration

// Configuration struct
type Configuration struct {
	Port                string `default:"8080" env:"WWC_PORT"`
	LogLevel            string `default:"info" env:"WWC_LOG_LEVEL"`
	S3                  S3
	RedisQueue          RedisQueue
	SentryDSN           string `env:"WWC_APP_SENTRY_DSN"`
	SessionTypeToStore  string `default:"remote" env:"WWC_SESSION_TYPE_TO_STORE"`
	DB                  DB
	LogConnectionErrors bool `default:"false" env:"WWC_LOG_CONNECTION_ERRORS"`
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
	Tag                   string `default:"wwcs-service" env:"WWC_REDIS_QUEUE_TAG"`
	URL                   string `default:"redis://localhost:6379/1" env:"WWC_REDIS_QUEUE_URL"`
	ConsumerPrefetchLimit int64  `default:"1000" env:"WWC_REDIS_QUEUE_CONSUMER_PREFETCH_LIMIT"`
	ConsumerPollDuration  int64  `default:"100" env:"WWC_REDIS_QUEUE_CONSUMER_POLL_DURATION"`
	RetryPrefetchLimit    int64  `default:"1000" env:"WWC_REDIS_QUEUE_RETRY_PREFETCH_LIMIT"`
	RetryPollDuration     int64  `default:"60000" env:"WWC_REDIS_QUEUE_RETRY_POLL_DURATION"`
}

type DB struct {
	Name string `default:"weni-webchat" env:"WWC_DB_NAME"`
	URI  string `default:"mongodb://admin:admin@localhost:27017/" env:"WWC_DB_URI"`
}

// Get all configs from env vars or config file
func Get() *Configuration {
	if configs == nil {
		config := Configuration{}
		log.Trace("Loading configs")
		settings := configor.Config{
			ENVPrefix: "WWC",
			Silent:    true,
		}

		if err := configor.New(&settings).Load(&config, "config.json"); err != nil {
			log.Fatal(err)
		}

		configs = &config
	}

	return configs
}

func Clear() {
	configs = nil
}
