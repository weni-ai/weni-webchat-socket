package websocket

import (
	"github.com/adjust/rmq/v4"
)

// App encapsulates application with resources.
type App struct {
	Pool          *Pool
	RMQConnection rmq.Connection
	OutgoingQueue rmq.Queue
}

// Create new App instance.
func NewApp(pool *Pool, rmqc rmq.Connection, oq rmq.Queue) *App {
	return &App{
		Pool:          pool,
		RMQConnection: rmqc,
		OutgoingQueue: oq,
	}
}
