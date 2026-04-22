package metric

import "github.com/prometheus/client_golang/prometheus"

// Service implements metric.UseCase interface
type Service struct {
	socketRegistrations *prometheus.HistogramVec
	openConnections     *prometheus.GaugeVec
	clientMessages      *prometheus.HistogramVec
	connectionAttempts  *prometheus.CounterVec
}

// NewPrometheusService returns a new metric service
func NewPrometheusService() (*Service, error) {
	socketRegistrations := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "socket_registrations",
		Help: "Registration count per channel, hostApi and origin",
	}, []string{"channel", "hostApi", "origin"})

	openConnections := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "open_connections",
		Help: "Open Connections count per channel, hostApi and origin",
	}, []string{"channel", "hostApi", "origin"})

	clientMessages := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "client_messages",
		Help: "Counter of client messages labeled by channel, hostApi, origin and status",
	}, []string{"channel", "hostApi", "origin", "status"})

	connectionAttempts := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "connection_attempts",
		Help: "Total WebSocket connection attempts on /ws labeled by origin and status",
	}, []string{"origin", "status"})

	s := &Service{
		socketRegistrations: socketRegistrations,
		openConnections:     openConnections,
		clientMessages:      clientMessages,
		connectionAttempts:  connectionAttempts,
	}
	err := prometheus.Register(s.socketRegistrations)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	err = prometheus.Register(s.openConnections)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	err = prometheus.Register(s.clientMessages)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	err = prometheus.Register(s.connectionAttempts)
	if err != nil && err.Error() != "duplicate metrics collector registration attempted" {
		return nil, err
	}

	return s, nil
}

// SaveSocketRegistration receive a *metric.SocketRegistration metric and save to a Histogram
func (s *Service) SaveSocketRegistration(sr *SocketRegistration) {
	s.socketRegistrations.WithLabelValues(sr.Channel, sr.HostAPI, sr.Origin).Observe(sr.Duration)
}

// IncOpenConnections receive a *metric.OpenConnection metric and increment to a Gauge
func (s *Service) IncOpenConnections(oc *OpenConnection) {
	s.openConnections.WithLabelValues(oc.Channel, oc.HostAPI, oc.Origin).Inc()
}

// IncOpenConnections receive a *metric.OpenConnection metric and decrement to a Gauge
func (s *Service) DecOpenConnections(oc *OpenConnection) {
	s.openConnections.WithLabelValues(oc.Channel, oc.HostAPI, oc.Origin).Dec()
}

// SaveClientMessages receive a *metric.ClientMessage metric and increment to a Gauge
func (s *Service) SaveClientMessages(cm *ClientMessage) {
	s.clientMessages.WithLabelValues(cm.Channel, cm.HostAPI, cm.Origin, cm.Status).Observe(cm.Duration)
}

// IncConnectionAttempts receive a *metric.ConnectionAttempt metric and increment the counter
// labeled by origin and status.
func (s *Service) IncConnectionAttempts(ca *ConnectionAttempt) {
	s.connectionAttempts.WithLabelValues(ca.Origin, ca.Status).Inc()
}
