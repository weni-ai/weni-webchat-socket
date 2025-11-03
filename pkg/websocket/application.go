package websocket

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/flows"
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
	FlowsClient            flows.IClient
}

// Create new App instance.
func NewApp(pool *ClientPool, rdb *redis.Client, mdb *mongo.Database, metrics *metric.Service, histories history.Service, clientM ClientManager, qconnM queue.Connection, fc flows.IClient) *App {
	return &App{
		ClientPool:             pool,
		RDB:                    rdb,
		MDB:                    mdb,
		Metrics:                metrics,
		Histories:              histories,
		ClientManager:          clientM,
		QueueConnectionManager: qconnM,
		FlowsClient:            fc,
	}
}

func (a *App) StartConnectionsHeartbeat() error {
	go func() {
        // Run a light background refresh far less frequently; individual connections refresh their TTL on ping
        for range time.Tick(time.Second * time.Duration(a.ClientManager.DefaultClientTTL()) * 5) {
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
