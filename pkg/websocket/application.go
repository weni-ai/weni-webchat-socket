package websocket

import (
	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
)

// App encapsulates application with resources.
type App struct {
	ClientPool    *ClientPool
	OutgoingQueue queue.Queue
	RDB           *redis.Client
	Metrics       *metric.Service
	Histories     history.Service
	ClientManager ClientManager
}

// Create new App instance.
func NewApp(pool *ClientPool, oq queue.Queue, rdb *redis.Client, metrics *metric.Service, histories history.Service, clientM ClientManager) *App {
	return &App{
		ClientPool:    pool,
		OutgoingQueue: oq,
		RDB:           rdb,
		Metrics:       metrics,
		Histories:     histories,
		ClientManager: clientM,
	}
}
