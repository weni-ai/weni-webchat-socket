package websocket

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/queue"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
)

// App encapsulates application with resources.
type App struct {
	ClientPool             *ClientPool
	RDB                    *redis.Client
	MDB                    *mongo.Database
	Metrics                *metric.Service
	Histories              history.Service
	ClientManager          ClientManager
	QueueConnectionManager queue.Connection
}

// Create new App instance.
func NewApp(pool *ClientPool, rdb *redis.Client, mdb *mongo.Database, metrics *metric.Service, histories history.Service, clientM ClientManager, qconnM queue.Connection) *App {
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
			clientsKeys := a.ClientPool.GetClientsKeys()
			pipe := a.RDB.Pipeline()
			for _, ck := range clientsKeys {
				clientConnectionKey := ClientConnectionKeyPrefix + ck
				pipe.Expire(context.Background(), clientConnectionKey, time.Second*time.Duration(a.ClientManager.DefaultClientTTL()))
			}
			if len(clientsKeys) > 0 {
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
