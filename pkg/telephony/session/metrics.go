package session

import (
	"github.com/ilhasoft/wwcs/pkg/metric"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// SessionMetrics exposes telephony-specific Prometheus metrics.
type SessionMetrics struct {
	callSetupDuration   prometheus.Histogram
	sttCommitLatency    prometheus.Histogram
	agentRoundtrip      prometheus.Histogram
	ttsBatchDuration    prometheus.Histogram
	bargeInLatency      prometheus.Histogram
	callTeardownTotal   *prometheus.CounterVec
	activeCalls         prometheus.Gauge
	queuedCalls         prometheus.Gauge
	baseMetrics         *metric.Service
}

// NewSessionMetrics registers telephony metrics, optionally wrapping the shared metric service.
func NewSessionMetrics(base *metric.Service) (*SessionMetrics, error) {
	sm := &SessionMetrics{
		baseMetrics: base,
		callSetupDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telephony_call_setup_duration_seconds",
			Help: "Time to establish a telephony call session through STT setup",
		}),
		sttCommitLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telephony_stt_commit_latency_seconds",
			Help: "Latency from audio to committed STT transcript",
		}),
		agentRoundtrip: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telephony_agent_roundtrip_seconds",
			Help: "Latency from committed transcript to first agent delta",
		}),
		ttsBatchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telephony_tts_batch_duration_seconds",
			Help: "Duration of a TTS batch synthesis",
		}),
		bargeInLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name: "telephony_bargein_latency_seconds",
			Help: "Latency from partial transcript to playback stop during barge-in",
		}),
		callTeardownTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "telephony_call_teardown_total",
			Help: "Total call teardown events by reason",
		}, []string{"reason"}),
		activeCalls: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "telephony_active_calls",
			Help: "Number of active (non-queued, non-ended) telephony calls",
		}),
		queuedCalls: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "telephony_queued_calls",
			Help: "Number of queued telephony calls waiting for capacity",
		}),
	}

	collectors := []prometheus.Collector{
		sm.callSetupDuration,
		sm.sttCommitLatency,
		sm.agentRoundtrip,
		sm.ttsBatchDuration,
		sm.bargeInLatency,
		sm.callTeardownTotal,
		sm.activeCalls,
		sm.queuedCalls,
	}

	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if err.Error() != "duplicate metrics collector registration attempted" {
				return nil, err
			}
		}
	}

	return sm, nil
}

// ObserveCallSetupDuration records call setup duration in seconds.
func (m *SessionMetrics) ObserveCallSetupDuration(seconds float64) {
	m.callSetupDuration.Observe(seconds)
}

// ObserveSTTCommitLatency records STT commit latency in seconds.
func (m *SessionMetrics) ObserveSTTCommitLatency(seconds float64) {
	m.sttCommitLatency.Observe(seconds)
}

// ObserveAgentRoundtrip records agent roundtrip latency in seconds.
func (m *SessionMetrics) ObserveAgentRoundtrip(seconds float64) {
	m.agentRoundtrip.Observe(seconds)
}

// ObserveTTSBatchDuration records TTS batch duration in seconds.
func (m *SessionMetrics) ObserveTTSBatchDuration(seconds float64) {
	m.ttsBatchDuration.Observe(seconds)
}

// ObserveBargeInLatency records barge-in stop latency in seconds.
func (m *SessionMetrics) ObserveBargeInLatency(seconds float64) {
	m.bargeInLatency.Observe(seconds)
}

// IncCallTeardown increments the teardown counter for the given reason.
func (m *SessionMetrics) IncCallTeardown(reason string) {
	m.callTeardownTotal.WithLabelValues(reason).Inc()
}

// SetActiveCalls sets the active call gauge.
func (m *SessionMetrics) SetActiveCalls(count float64) {
	m.activeCalls.Set(count)
}

// SetQueuedCalls sets the queued call gauge.
func (m *SessionMetrics) SetQueuedCalls(count float64) {
	m.queuedCalls.Set(count)
}

// TeardownCount returns the teardown counter for a reason (for tests).
func (m *SessionMetrics) TeardownCount(reason string) float64 {
	if m == nil {
		return 0
	}
	return testutil.ToFloat64(m.callTeardownTotal.WithLabelValues(reason))
}

// HistogramHasSample reports whether a histogram has at least one observation (for tests).
func (m *SessionMetrics) HistogramHasSample(h prometheus.Histogram) bool {
	if m == nil {
		return false
	}
	metric := &dto.Metric{}
	if err := h.(prometheus.Metric).Write(metric); err != nil {
		return false
	}
	return metric.GetHistogram().GetSampleCount() > 0
}

// HasObservedSetupDuration reports whether call setup duration was recorded (for tests).
func (m *SessionMetrics) HasObservedSetupDuration() bool {
	return m.HistogramHasSample(m.callSetupDuration)
}

// HasObservedSTTCommitLatency reports whether STT commit latency was recorded (for tests).
func (m *SessionMetrics) HasObservedSTTCommitLatency() bool {
	return m.HistogramHasSample(m.sttCommitLatency)
}

// HasObservedAgentRoundtrip reports whether agent roundtrip latency was recorded (for tests).
func (m *SessionMetrics) HasObservedAgentRoundtrip() bool {
	return m.HistogramHasSample(m.agentRoundtrip)
}

// HasObservedTTSBatchDuration reports whether TTS batch duration was recorded (for tests).
func (m *SessionMetrics) HasObservedTTSBatchDuration() bool {
	return m.HistogramHasSample(m.ttsBatchDuration)
}

// HasObservedBargeInLatency reports whether barge-in latency was recorded (for tests).
func (m *SessionMetrics) HasObservedBargeInLatency() bool {
	return m.HistogramHasSample(m.bargeInLatency)
}
