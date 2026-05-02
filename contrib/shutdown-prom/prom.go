// Package shutdownprom provides a shutdown.Observer that emits Prometheus
// metrics for shutdown phase + handler events.
//
// Three metrics are exported:
//
//   - shutdown_phase_duration_seconds (histogram, label: phase)
//   - shutdown_handler_duration_seconds (histogram, labels: phase, name, status)
//   - shutdown_handlers_total (counter, labels: phase, name, status="ok|error")
//
// Pass your own Registerer to expose them on your existing /metrics scrape
// endpoint, or pass nil to use prometheus.DefaultRegisterer.
package shutdownprom

import (
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/ubgo/shutdown"
)

// Metrics bundles the Prometheus collectors created by this package. Useful
// when callers want to register them on a custom Registerer.
type Metrics struct {
	PhaseDuration   *prometheus.HistogramVec
	HandlerDuration *prometheus.HistogramVec
	HandlerCounter  *prometheus.CounterVec
}

// NewMetrics constructs the collectors and registers them with reg. A nil
// reg falls back to prometheus.DefaultRegisterer.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}
	m := &Metrics{
		PhaseDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "shutdown_phase_duration_seconds",
			Help:    "Time spent inside each shutdown phase.",
			Buckets: prometheus.DefBuckets,
		}, []string{"phase"}),
		HandlerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "shutdown_handler_duration_seconds",
			Help:    "Time spent inside each shutdown handler.",
			Buckets: prometheus.DefBuckets,
		}, []string{"phase", "name", "status"}),
		HandlerCounter: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "shutdown_handlers_total",
			Help: "Count of shutdown handler executions by outcome.",
		}, []string{"phase", "name", "status"}),
	}
	reg.MustRegister(m.PhaseDuration, m.HandlerDuration, m.HandlerCounter)
	return m
}

// Observer wires a Metrics bundle into a shutdown.Observer. Pass nil to
// have the observer construct a default Metrics on prometheus.DefaultRegisterer.
func Observer(m *Metrics) shutdown.Observer {
	if m == nil {
		m = NewMetrics(nil)
	}
	return shutdown.Observer{
		OnSignal: func(_ os.Signal) {},
		OnPhaseStart: func(_ shutdown.Phase, _ int) {},
		OnPhaseEnd: func(p shutdown.Phase, dur time.Duration, _ []error) {
			m.PhaseDuration.WithLabelValues(p.String()).Observe(dur.Seconds())
		},
		OnHandlerStart: func(_ string, _ shutdown.Phase) {},
		OnHandlerEnd: func(name string, p shutdown.Phase, dur time.Duration, err error) {
			status := "ok"
			if err != nil {
				status = "error"
			}
			m.HandlerDuration.WithLabelValues(p.String(), name, status).Observe(dur.Seconds())
			m.HandlerCounter.WithLabelValues(p.String(), name, status).Inc()
		},
		OnComplete: func(_ time.Duration, _ error) {},
	}
}
