// Copyright 2024 Canonical.

// The servermon package is used to update statistics used
// for monitoring the API server.
package servermon

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	AuthenticationFailCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "auth",
		Name:      "failure_total",
		Help:      "The number of failed authentications.",
	}, []string{"method"})
	AuthenticationSuccessCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "auth",
		Name:      "success_total",
		Help:      "The number of successful authentications.",
	}, []string{"method"})
	DBQueryDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "db",
		Name:      "query_duration_seconds",
		Help:      "Histogram of database query time in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method"})
	DBQueryErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "db",
		Name:      "error_total",
		Help:      "The number of database errors.",
	}, []string{"method"})
	OpenFGACallDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "openfga",
		Name:      "call_duration_seconds",
		Help:      "Histogram of openfga call time in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method"})
	OpenFGACallErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "openfga",
		Name:      "error_total",
		Help:      "The number of openfga call errors.",
	}, []string{"method"})
	VaultCallDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "vault",
		Name:      "call_duration_seconds",
		Help:      "Histogram of vault call time in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method"})
	VaultCallErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "vault",
		Name:      "error_total",
		Help:      "The number of vault call errors.",
	}, []string{"method"})
	JujuCallDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "juju",
		Name:      "call_duration_seconds",
		Help:      "Histogram of juju call time in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"facade", "method", "controller"})
	JujuPingDurationHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "juju",
		Name:      "ping_duration_seconds",
		Help:      "Histogram of juju ping time in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"controller"})
	JujuCallErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "juju",
		Name:      "error_total",
		Help:      "The number of juju call errors.",
	}, []string{"facade", "method", "controller"})
	ConcurrentWebsocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "jimm",
		Subsystem: "websocket",
		Name:      "concurrent_connections",
		Help:      "The number of concurrent websocket connections",
	})
	ModelsCreatedCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "websocket",
		Name:      "models_created_total",
		Help:      "The number of models created.",
	})
	ModelsCreatedFailCount = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "websocket",
		Name:      "models_created_fail_total",
		Help:      "The number of fails attempting to create models.",
	})
	MonitorDeltasReceivedCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "monitor",
		Name:      "deltas_received_total",
		Help:      "The number of watcher deltas received.",
	}, []string{"controller"})
	MonitorErrorsCount = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jimm",
		Subsystem: "monitor",
		Name:      "errors_total",
		Help:      "The number of monitoring errors found.",
	}, []string{"controller"})
	WebsocketRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "websocket",
		Name:      "request_duration_seconds",
		Help:      "The duration of a websocket request in seconds.",
	}, []string{"type", "action"})
	ModelCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jimm",
		Subsystem: "system",
		Name:      "model",
		Help:      "The number of models managed per controller attached to JIMM.",
	}, []string{"controller"})
	ControllerCount = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "jimm",
		Subsystem: "system",
		Name:      "controller",
		Help:      "The number of controllers managed by JIMM.",
	})
	ResponseTimeHistogram = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "The duration of handling an HTTP request in seconds.",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"route", "method", "status_code"})
)

// DurationObserver returns a function that, when run with `defer` will
// record the duration of the parent function's execution.
// Durations are observer as microseconds.
func DurationObserver(m *prometheus.HistogramVec, labelValues ...string) func() {
	start := time.Now()
	return func() {
		m.WithLabelValues(labelValues...).Observe(time.Since(start).Seconds())
	}
}

// ErrorCount increases the specified counter if the error is not nil.
func ErrorCounter(m *prometheus.CounterVec, err *error, labelValues ...string) {
	if *err == nil {
		return
	}

	m.WithLabelValues(labelValues...).Inc()
}
