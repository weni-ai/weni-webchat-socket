package websocket

import (
	"github.com/adjust/rmq/v4"
	"github.com/go-redis/redis/v8"
)

// App encapsulates application with resources.
type App struct {
	Pool          *Pool
	OutgoingQueue rmq.Queue
	RDB           *redis.Client
}

// Create new App instance.
func NewApp(pool *Pool, oq rmq.Queue, rdb *redis.Client) *App {
	return &App{
		Pool:          pool,
		OutgoingQueue: oq,
		RDB:           rdb,
	}
}
