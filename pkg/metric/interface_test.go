package metric

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetricInterface(t *testing.T) {
	socketRegistrationMetric := NewSocketRegistration(
		"asdf-asdf-asdf-asdf",
		"localhost",
		"http://localhost:9080",
		0.1,
	)
	assert.NotNil(t, socketRegistrationMetric)

	openConnectionMetric := NewOpenConnection(
		"asdf-asdf-asdf-asdf",
		"localhost",
		"http://localhost:9080",
	)
	assert.NotNil(t, openConnectionMetric)

	clientMessageMetric := NewClientMessage(
		"asdf-asdf-asdf-asdf",
		"localhost",
		"http://localhost:9080",
		"200",
		0.1,
	)

	assert.NotNil(t, clientMessageMetric)

	connectionAttemptMetric := NewConnectionAttempt(
		"http://localhost:9080",
		ConnectionAttemptStatusUpgraded,
	)
	assert.NotNil(t, connectionAttemptMetric)
	assert.Equal(t, "http://localhost:9080", connectionAttemptMetric.Origin)
	assert.Equal(t, ConnectionAttemptStatusUpgraded, connectionAttemptMetric.Status)
}
