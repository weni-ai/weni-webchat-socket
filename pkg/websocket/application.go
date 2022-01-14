package websocket

import (
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
)

// App encapsulates application with resources.
type App struct {
	Pool          *Pool
	OutgoingQueue queue.Queue
	RDB           *redis.Client
	Metrics       *metric.Service
}

// Create new App instance.
func NewApp(pool *Pool, oq queue.Queue, rdb *redis.Client, metrics *metric.Service) *App {
	return &App{
		Pool:          pool,
		OutgoingQueue: oq,
		RDB:           rdb,
		Metrics:       metrics,
	}
}
