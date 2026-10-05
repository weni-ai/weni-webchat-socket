package health

import (
	"testing"
	"time"

	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/stretchr/testify/assert"
)

type stubMetrics struct {
	observations []*metric.HealthcheckLatency
}

func (s *stubMetrics) SaveSocketRegistration(_ *metric.SocketRegistration) {}

func (s *stubMetrics) IncOpenConnections(_ *metric.OpenConnection) {}

func (s *stubMetrics) DecOpenConnections(_ *metric.OpenConnection) {}

func (s *stubMetrics) SaveClientMessages(_ *metric.ClientMessage) {}

func (s *stubMetrics) IncConnectionAttempts(_ *metric.ConnectionAttempt) {}

func (s *stubMetrics) ObserveHealthcheck(hc *metric.HealthcheckLatency) {
	s.observations = append(s.observations, hc)
}

func (s *stubMetrics) IncUTMSends(_ *metric.UTMSend) {}

func TestRecordLatencies(t *testing.T) {
	stub := &stubMetrics{}
	RecordLatencies(stub, CheckLatencies{
		Redis:          10 * time.Millisecond,
		MongoDB:        20 * time.Millisecond,
		Total:          35 * time.Millisecond,
		RedisChecked:   true,
		MongoDBChecked: true,
	})

	assert.Len(t, stub.observations, 3)
	assert.Equal(t, metric.HealthcheckDependencyRedis, stub.observations[0].Dependency)
	assert.InDelta(t, 0.01, stub.observations[0].Duration, 0.001)
	assert.Equal(t, metric.HealthcheckDependencyMongoDB, stub.observations[1].Dependency)
	assert.InDelta(t, 0.02, stub.observations[1].Duration, 0.001)
	assert.Equal(t, metric.HealthcheckDependencyTotal, stub.observations[2].Dependency)
	assert.InDelta(t, 0.035, stub.observations[2].Duration, 0.001)
}

func TestRecordLatenciesSkipsUncheckedDependencies(t *testing.T) {
	stub := &stubMetrics{}
	RecordLatencies(stub, CheckLatencies{
		Redis:        2 * time.Second,
		Total:        2 * time.Second,
		RedisChecked: true,
	})

	assert.Len(t, stub.observations, 2)
	assert.Equal(t, metric.HealthcheckDependencyRedis, stub.observations[0].Dependency)
	assert.Equal(t, metric.HealthcheckDependencyTotal, stub.observations[1].Dependency)
}

func TestRecordLatenciesNilMetrics(t *testing.T) {
	RecordLatencies(nil, CheckLatencies{
		Redis:          time.Millisecond,
		MongoDB:        time.Millisecond,
		Total:          time.Millisecond,
		RedisChecked:   true,
		MongoDBChecked: true,
	})
}
