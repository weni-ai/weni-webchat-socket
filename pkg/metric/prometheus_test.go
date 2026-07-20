package metric

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
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
