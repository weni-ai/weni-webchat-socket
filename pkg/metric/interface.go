package metric

// Connection attempt status values used as the `status` label of the
// connection_attempts counter.
const (
	ConnectionAttemptStatusUpgraded        = "upgraded"
	ConnectionAttemptStatusProtocolInvalid = "protocol_invalid"
	ConnectionAttemptStatusUpgradeFailed   = "upgrade_failed"
)

// Healthcheck dependency values used as the `dependency` label of the
// healthcheck_duration_seconds histogram.
const (
	HealthcheckDependencyRedis   = "redis"
	HealthcheckDependencyMongoDB = "mongodb"
	HealthcheckDependencyTotal   = "total"
)

// ConnectionAttempt represents a WebSocket connection attempt on /ws,
// regardless of whether the upgrade succeeded.
type ConnectionAttempt struct {
	Origin string
	Status string
}

// NewConnectionAttempt returns new ConnectionAttempt metric struct value representation.
func NewConnectionAttempt(origin string, status string) *ConnectionAttempt {
	return &ConnectionAttempt{origin, status}
}

// SocketRegistration represents a socket registration histogram metric.
type SocketRegistration struct {
	Channel  string
	HostAPI  string
	Origin   string
	Duration float64
}

// NewSocketRegistration returns new SocketRegistration metric struct value representation.
func NewSocketRegistration(channel string, hostAPI string, origin string, duration float64) *SocketRegistration {
	return &SocketRegistration{channel, hostAPI, origin, duration}
}

// OpenConnection represents an open connection metric.
type OpenConnection struct {
	Channel string
	HostAPI string
	Origin  string
}

// NewOpenConnection returns new OpenConnection metric struct value representation.
func NewOpenConnection(channel string, hostAPI string, origin string) *OpenConnection {
	return &OpenConnection{channel, hostAPI, origin}
}

// ClientMessage represents a client message metric.
type ClientMessage struct {
	Channel  string
	HostAPI  string
	Origin   string
	Status   string
	Duration float64
}

// NewOpenConnection returns new OpenConnection metric struct value representation.
func NewClientMessage(channel string, hostAPI string, origin string, status string, duration float64) *ClientMessage {
	return &ClientMessage{channel, hostAPI, origin, status, duration}
}

// HealthcheckLatency represents a dependency healthcheck duration histogram metric.
type HealthcheckLatency struct {
	Dependency string
	Duration   float64
}

// NewHealthcheckLatency returns new HealthcheckLatency metric struct value representation.
func NewHealthcheckLatency(dependency string, duration float64) *HealthcheckLatency {
	return &HealthcheckLatency{dependency, duration}
}

// UTM send status values used as the `status` label of the utm_sends counter.
const (
	UTMSendStatusSent            = "sent"
	UTMSendStatusError           = "error"
	UTMSendStatusInvalidSource   = "invalid_source"
	UTMSendStatusMissingFields   = "missing_fields"
	UTMSendStatusNotRegistered   = "not_registered"
	UTMSendStatusFeatureDisabled = "feature_disabled"
)

// UTMSend represents a send_utm attempt outcome, labeled by utm_source and status.
type UTMSend struct {
	UTMSource string
	Status    string
}

// NewUTMSend returns new UTMSend metric struct value representation.
func NewUTMSend(utmSource string, status string) *UTMSend {
	return &UTMSend{utmSource, status}
}

// UseCase encapsulates interface definitions
type UseCase interface {
	SaveSocketRegistration(sr *SocketRegistration)
	IncOpenConnections(oc *OpenConnection)
	DecOpenConnections(oc *OpenConnection)
	SaveClientMessages(cm *ClientMessage)
	IncConnectionAttempts(ca *ConnectionAttempt)
	ObserveHealthcheck(hc *HealthcheckLatency)
	IncUTMSends(us *UTMSend)
}
