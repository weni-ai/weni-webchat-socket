package websocket

import (
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/ilhasoft/wwcs/pkg/flows"
	"github.com/ilhasoft/wwcs/pkg/history"
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/ilhasoft/wwcs/pkg/starters"
	"github.com/ilhasoft/wwcs/pkg/streams"
	"go.mongodb.org/mongo-driver/mongo"
	"golang.org/x/sync/semaphore"
)

// App encapsulates application with resources.
type App struct {
	ClientPool      *ClientPool
	RDB             *redis.Client
	MDB             *mongo.Database
	Metrics         *metric.Service
	Histories       history.Service
	ClientManager   ClientManager
	Router          streams.Router
	PodID           string
	FlowsClient    flows.IClient
	StartersService starters.StartersService
	StartersSem     *semaphore.Weighted
	// StartersInFlight tracks per-client in-flight starters requests.
	// Key: client ID, Value: request fingerprint (account:linkText).
	// Prevents a single client from spawning multiple concurrent Lambda
	// invocations and deduplicates identical rapid requests.
	StartersInFlight sync.Map
}

// NewApp creates a new App instance.
func NewApp(pool *ClientPool, rdb *redis.Client, mdb *mongo.Database, metrics *metric.Service, histories history.Service, clientM ClientManager, router streams.Router, podID string, fc flows.IClient) *App {
	return &App{
		ClientPool:    pool,
		RDB:           rdb,
		MDB:           mdb,
		Metrics:       metrics,
		Histories:     histories,
		ClientManager: clientM,
		Router:        router,
		PodID:         podID,
		FlowsClient:  fc,
	}
}
