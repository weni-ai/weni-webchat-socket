package websocket

import "github.com/ilhasoft/wwcs/pkg/queue"

// App encapsulates application with resources.
type App struct {
	Pool       *Pool
	QOProducer queue.Producer
	QIProducer queue.Producer
}

// Create new App instance.
func NewApp(pool *Pool, qop queue.Producer, qip queue.Producer) *App {
	return &App{
		Pool:       pool,
		QOProducer: qop,
		QIProducer: qip,
	}
}
