package metric

type SocketRegistration struct {
	Channel string
	HostAPI string
	Origin  string
}

func NewSocketRegistration(channel string, hostAPI string, origin string) *SocketRegistration {
	return &SocketRegistration{channel, hostAPI, origin}
}

type OpenConnection struct {
	Channel string
	HostAPI string
	Origin  string
}

func NewOpenConnection(channel string, hostAPI string, origin string) *OpenConnection {
	return &OpenConnection{channel, hostAPI, origin}
}

type ClientMessage struct {
	Channel string
	HostAPI string
	Origin  string
	Status  string
}

func NewClientMessage(channel string, hostAPI string, origin string, status string) *ClientMessage {
	return &ClientMessage{channel, hostAPI, origin, status}
}

type UseCase interface {
	SaveSocketRegistration(sr *SocketRegistration)
	IncOpenConnections(oc *OpenConnection)
	DecOpenConnections(oc *OpenConnection)
	SaveClientMessages(cm *ClientMessage)
}
