package metric

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

// UseCase encapsulates interface definitions
type UseCase interface {
	SaveSocketRegistration(sr *SocketRegistration)
	IncOpenConnections(oc *OpenConnection)
	DecOpenConnections(oc *OpenConnection)
	SaveClientMessages(cm *ClientMessage)
}
