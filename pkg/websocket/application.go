package websocket

import "github.com/ilhasoft/wwcs/pkg/queue"

type App struct {
	Pool       *Pool
	QOProducer queue.Producer
	QIProducer queue.Producer
}

func NewApp(pool *Pool, qop queue.Producer, qip queue.Producer) *App {
	return &App{
		Pool:       pool,
		QOProducer: qop,
		QIProducer: qip,
	}
}
