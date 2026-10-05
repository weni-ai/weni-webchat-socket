package metric

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestMetricService(t *testing.T) {
	metricsService, err := NewPrometheusService()
	assert.NoError(t, err)
	assert.NotNil(t, metricsService)
}

func TestIncConnectionAttempts(t *testing.T) {
	s, err := NewPrometheusService()
	assert.NoError(t, err)
	assert.NotNil(t, s)

	origin := "http://example.test"

	// Snapshot counts first — other tests in this suite may share the same
	// global registry and increment these series too.
	baseUpgraded := testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusUpgraded))
	baseProtoInvalid := testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusProtocolInvalid))
	baseUpgradeFailed := testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusUpgradeFailed))

	s.IncConnectionAttempts(NewConnectionAttempt(origin, ConnectionAttemptStatusUpgraded))
	s.IncConnectionAttempts(NewConnectionAttempt(origin, ConnectionAttemptStatusUpgraded))
	s.IncConnectionAttempts(NewConnectionAttempt(origin, ConnectionAttemptStatusProtocolInvalid))
	s.IncConnectionAttempts(NewConnectionAttempt(origin, ConnectionAttemptStatusUpgradeFailed))

	assert.Equal(t, baseUpgraded+2, testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusUpgraded)))
	assert.Equal(t, baseProtoInvalid+1, testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusProtocolInvalid)))
	assert.Equal(t, baseUpgradeFailed+1, testutil.ToFloat64(s.connectionAttempts.WithLabelValues(origin, ConnectionAttemptStatusUpgradeFailed)))
}

func TestObserveHealthcheck(t *testing.T) {
	s, err := NewPrometheusService()
	assert.NoError(t, err)
	assert.NotNil(t, s)

	baseRedis := healthcheckSampleCount(t, s, HealthcheckDependencyRedis)
	baseMongoDB := healthcheckSampleCount(t, s, HealthcheckDependencyMongoDB)
	baseTotal := healthcheckSampleCount(t, s, HealthcheckDependencyTotal)

	s.ObserveHealthcheck(NewHealthcheckLatency(HealthcheckDependencyRedis, 0.01))
	s.ObserveHealthcheck(NewHealthcheckLatency(HealthcheckDependencyMongoDB, 0.02))
	s.ObserveHealthcheck(NewHealthcheckLatency(HealthcheckDependencyTotal, 0.035))

	assert.Equal(t, baseRedis+1, healthcheckSampleCount(t, s, HealthcheckDependencyRedis))
	assert.Equal(t, baseMongoDB+1, healthcheckSampleCount(t, s, HealthcheckDependencyMongoDB))
	assert.Equal(t, baseTotal+1, healthcheckSampleCount(t, s, HealthcheckDependencyTotal))
}

func healthcheckSampleCount(t *testing.T, s *Service, dependency string) uint64 {
	t.Helper()

	metric, err := s.healthcheckDurations.GetMetricWithLabelValues(dependency)
	assert.NoError(t, err)

	promMetric, ok := metric.(prometheus.Metric)
	assert.True(t, ok)

	var dtoMetric dto.Metric
	assert.NoError(t, promMetric.Write(&dtoMetric))
	return dtoMetric.GetHistogram().GetSampleCount()
}

func TestIncUTMSends(t *testing.T) {
	s, err := NewPrometheusService()
	assert.NoError(t, err)
	assert.NotNil(t, s)

	utmSource := "cx_shopping_assistant"

	// Snapshot counts first — other tests in this suite may share the same
	// global registry and increment these series too.
	baseSent := testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusSent))
	baseError := testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusError))
	baseInvalidSource := testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusInvalidSource))

	s.IncUTMSends(NewUTMSend(utmSource, UTMSendStatusSent))
	s.IncUTMSends(NewUTMSend(utmSource, UTMSendStatusSent))
	s.IncUTMSends(NewUTMSend(utmSource, UTMSendStatusError))
	s.IncUTMSends(NewUTMSend(utmSource, UTMSendStatusInvalidSource))

	assert.Equal(t, baseSent+2, testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusSent)))
	assert.Equal(t, baseError+1, testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusError)))
	assert.Equal(t, baseInvalidSource+1, testutil.ToFloat64(s.utmSends.WithLabelValues(utmSource, UTMSendStatusInvalidSource)))
}
