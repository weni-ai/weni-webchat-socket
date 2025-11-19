package health

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/grpc"
	grpcHealth "google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// Monitor manages gRPC health status and dependency checks.
type Monitor struct {
	healthServer *grpcHealth.Server
	serviceName  string
	cancel       context.CancelFunc
	ticker       *time.Ticker
	rdb          *redis.Client
	mdb          *mongo.Database
}

// Start registers the standard gRPC health service and launches a background
// monitor that toggles SERVING/NOT_SERVING based on Redis and MongoDB checks.
func Start(grpcServer *grpc.Server, rdb *redis.Client, mdb *mongo.Database, serviceName string) *Monitor {
	hs := grpcHealth.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, hs)

	// Initially mark as SERVING
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	if serviceName != "" {
		hs.SetServingStatus(serviceName, grpc_health_v1.HealthCheckResponse_SERVING)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ticker := time.NewTicker(5 * time.Second)

	mon := &Monitor{
		healthServer: hs,
		serviceName:  serviceName,
		cancel:       cancel,
		ticker:       ticker,
		rdb:          rdb,
		mdb:          mdb,
	}

	go mon.loop(ctx)
	return mon
}

// StartMonitor is a convenient alias to Start for fluent usage in main packages.
// It returns a Monitor already registered and running.
func StartMonitor(grpcServer *grpc.Server, rdb *redis.Client, mdb *mongo.Database, serviceName string) *Monitor {
	return Start(grpcServer, rdb, mdb, serviceName)
}

// Stop marks NOT_SERVING and stops the monitor goroutine.
func (m *Monitor) Stop() {
	if m == nil {
		return
	}
	// Mark health as NOT_SERVING
	m.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	if m.serviceName != "" {
		m.healthServer.SetServingStatus(m.serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	}
	// Stop background loop
	m.cancel()
	if m.ticker != nil {
		m.ticker.Stop()
	}
}

func (m *Monitor) loop(ctx context.Context) {
	isServing := true
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.ticker.C:
			depsOK := m.checkDependencies()
			if depsOK && !isServing {
				m.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
				if m.serviceName != "" {
					m.healthServer.SetServingStatus(m.serviceName, grpc_health_v1.HealthCheckResponse_SERVING)
				}
				isServing = true
				log.Info("Health monitor: dependencies restored, status SERVING")
			} else if !depsOK && isServing {
				m.healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				if m.serviceName != "" {
					m.healthServer.SetServingStatus(m.serviceName, grpc_health_v1.HealthCheckResponse_NOT_SERVING)
				}
				isServing = false
				log.Warn("Health monitor: dependencies failing, status NOT_SERVING")
			}
		}
	}
}

func (m *Monitor) checkDependencies() bool {
	if m.rdb == nil || m.mdb == nil {
		log.Warn("Health monitor: dependencies are not initialized")
		return false
	}
	// Check Redis
	rc, rcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer rcancel()
	if err := m.rdb.Ping(rc).Err(); err != nil {
		log.WithError(errors.WithStack(err)).Warn("Health monitor: Redis ping failed")
		return false
	}

	// Check MongoDB
	mc, mcancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer mcancel()
	if err := m.mdb.Client().Ping(mc, nil); err != nil {
		log.WithError(errors.WithStack(err)).Warn("Health monitor: MongoDB ping failed")
		return false
	}

	return true
}
