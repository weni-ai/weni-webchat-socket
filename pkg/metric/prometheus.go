package metric

import "github.com/prometheus/client_golang/prometheus"

type Service struct {
	registrationCounter   *prometheus.CounterVec
	openConnectionsGauge  *prometheus.GaugeVec
	clientMessagesCounter *prometheus.CounterVec
}

func NewPrometheusService() (*Service, error) {
	socketRegistrationsCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "socket_registration_counter",
		Help: "Registration count per channel, hostApi and origin",
	}, []string{"channel", "hostApi", "origin"})

	openConnectionsGauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "open_connections_gauge",
		Help: "Open Connections count per channel, hostApi and origin",
	}, []string{"channel", "hostApi", "origin"})

	clientMessagesCounter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "client_messages_counter",
		Help: "Counter of client messages labeled by channel, hostApi, origin and status",
	}, []string{"channel", "hostApi", "origin", "status"})

	s := &Service{
		registrationCounter:   socketRegistrationsCounter,
		openConnectionsGauge:  openConnectionsGauge,
		clientMessagesCounter: clientMessagesCounter,
	}
	err := prometheus.Register(s.registrationCounter)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	err = prometheus.Register(s.openConnectionsGauge)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	err = prometheus.Register(s.clientMessagesCounter)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	return s, nil
}

func (s *Service) SaveSocketRegistration(sr *SocketRegistration) {
	s.registrationCounter.WithLabelValues(sr.Channel, sr.HostAPI, sr.Origin).Inc()
}

func (s *Service) IncOpenConnections(oc *OpenConnection) {
	s.openConnectionsGauge.WithLabelValues(oc.Channel, oc.HostAPI, oc.Origin).Inc()
}
func (s *Service) DecOpenConnections(oc *OpenConnection) {
	s.openConnectionsGauge.WithLabelValues(oc.Channel, oc.HostAPI, oc.Origin).Dec()
}

func (s *Service) SaveClientMessages(cm *ClientMessage) {
	s.clientMessagesCounter.WithLabelValues(cm.Channel, cm.HostAPI, cm.Origin, cm.Status).Inc()
}
