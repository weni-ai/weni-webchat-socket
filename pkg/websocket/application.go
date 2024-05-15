package websocket

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
	log "github.com/sirupsen/logrus"
)

// App encapsulates application with resources.
type App struct {
	ClientPool             *ClientPool
	RDB                    *redis.Client
	Metrics                *metric.Service
	Histories              history.Service
	ClientManager          ClientManager
	QueueConnectionManager queue.Connection
}

// Create new App instance.
func NewApp(pool *ClientPool, rdb *redis.Client, metrics *metric.Service, histories history.Service, clientM ClientManager, qconnM queue.Connection) *App {
	return &App{
		ClientPool:             pool,
		RDB:                    rdb,
		Metrics:                metrics,
		Histories:              histories,
		ClientManager:          clientM,
		QueueConnectionManager: qconnM,
	}
}

func (a *App) StartConnectionsHeartbeat() error {
	go func() {
		for range time.Tick(time.Second * time.Duration(a.ClientManager.DefaultClientTTL()) / 2) {
			clients := a.ClientPool.GetClients()
			pipe := a.RDB.Pipeline()
			for ck := range clients {
				clientConnectionKey := ClientConnectionKeyPrefix + ck
				pipe.Expire(context.Background(), clientConnectionKey, time.Second*time.Duration(a.ClientManager.DefaultClientTTL()))
			}
			if len(clients) > 0 {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(a.ClientManager.DefaultClientTTL()))
				_, err := pipe.Exec(ctx)
				if err != nil {
					log.Error(err)
				}
				cancel()
			}
		}
	}()

	return nil
}
