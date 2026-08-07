package health

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"go.mongodb.org/mongo-driver/mongo"
)

func PingRedis(ctx context.Context, rdb *redis.Client) (time.Duration, error) {
	start := time.Now()
	err := rdb.Ping(ctx).Err()
	return time.Since(start), err
}

func PingMongo(ctx context.Context, mdb *mongo.Database) (time.Duration, error) {
	start := time.Now()
	err := mdb.Client().Ping(ctx, nil)
	return time.Since(start), err
}

// CheckLatencies holds measured healthcheck durations. Only dependencies that
// were actually pinged should have their corresponding Checked flag set.
type CheckLatencies struct {
	Redis          time.Duration
	MongoDB        time.Duration
	Total          time.Duration
	RedisChecked   bool
	MongoDBChecked bool
}

func RecordLatencies(metrics metric.UseCase, latencies CheckLatencies) {
	if metrics == nil {
		return
	}
	if latencies.RedisChecked {
		metrics.ObserveHealthcheck(metric.NewHealthcheckLatency(metric.HealthcheckDependencyRedis, latencies.Redis.Seconds()))
	}
	if latencies.MongoDBChecked {
		metrics.ObserveHealthcheck(metric.NewHealthcheckLatency(metric.HealthcheckDependencyMongoDB, latencies.MongoDB.Seconds()))
	}
	metrics.ObserveHealthcheck(metric.NewHealthcheckLatency(metric.HealthcheckDependencyTotal, latencies.Total.Seconds()))
}
