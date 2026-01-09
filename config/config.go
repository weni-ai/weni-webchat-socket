package config

import (
	"time"

	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

var configs *Configuration

// Configuration struct
type Configuration struct {
	Port       string `default:"8080" env:"WWC_PORT"`
	LogLevel   string `default:"info" env:"WWC_LOG_LEVEL"`
	S3         S3
	RedisQueue RedisQueue
	SentryDSN  string `env:"WWC_APP_SENTRY_DSN"`
	DB         DB
	JWT        JWT

	RestrictDomains bool `default:"false" env:"WWC_RESTRICT_DOMAINS"`

	FlowsURL        string `default:"https://flows.weni.ai" env:"WWC_FLOWS_URL"`
	MemCacheTimeout int64  `default:"5" env:"WWC_MEM_CACHE_TIMEOUT"`

	// gRPC server configuration (for message streaming from Nexus)
	GRPCServerAddr string `default:":50051" env:"GRPC_SERVER_ADDR"`
}

type JWT struct {
	PrivateKey     string `env:"WWC_JWT_PRIVATE_KEY"`
	ExpirationMins int64  `default:"60" env:"WWC_JWT_EXPIRATION_MINS"`
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
	Timeout               int64  `default:"15" env:"WWC_REDIS_TIMEOUT"`
	MaxRetries            int64  `default:"3" env:"WWC_REDIS_MAX_RETRIES"`
	RetentionLimit        int64  `default:"10" env:"WWC_REDIS_QUEUE_RETENTION_LIMIT"`
	ClientTTL             int64  `default:"12" env:"WWC_REDIS_CLIENT_TTL"`
	HealthcheckTimeout    int64  `default:"10" env:"WWC_REDIS_HEALTHCHECK_TIMEOUT"`
	CleanBatchSize        int64  `default:"500" env:"WWC_REDIS_QUEUE_CLEAN_BATCH_SIZE"`

	// Streams (Redis Streams) configuration
	StreamsMaxLen         int64 `default:"20000" env:"WWC_REDIS_STREAMS_MAX_LEN"`
	StreamsReadCount      int64 `default:"100" env:"WWC_REDIS_STREAMS_READ_COUNT"`
	StreamsBlockMs        int64 `default:"5000" env:"WWC_REDIS_STREAMS_BLOCK_MS"`
	StreamsClaimIdleMs    int64 `default:"10000" env:"WWC_REDIS_STREAMS_CLAIM_IDLE_MS"`
	JanitorIntervalMs     int64 `default:"10000" env:"WWC_REDIS_JANITOR_INTERVAL_MS"`
	JanitorLeaseMs        int64 `default:"30000" env:"WWC_REDIS_JANITOR_LEASE_MS"`
	StreamsRetentionMs    int64 `default:"0" env:"WWC_REDIS_STREAMS_RETENTION_MS"`        // 0 disables time-based trimming
	StreamsMaxPendingAgMs int64 `default:"120000" env:"WWC_REDIS_STREAMS_MAX_PENDING_MS"` // max age (ms) before pending messages are dropped; 0 disables
}

type DB struct {
	Name               string        `default:"weni-webchat" env:"WWC_DB_NAME"`
	URI                string        `default:"mongodb://admin:admin@localhost:27017/" env:"WWC_DB_URI"`
	ContextTimeout     time.Duration `default:"15" env:"WWC_DB_CONTEXT_TIMEOUT"`
	HealthcheckTimeout int64         `default:"15" env:"WWC_DB_HEALTHCHECK_TIMEOUT"`
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
