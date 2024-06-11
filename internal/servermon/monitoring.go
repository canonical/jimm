// Copyright 2016 Canonical Ltd.

// The servermon package is used to update statistics used
// for monitoring the API server.
package servermon

import (
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
	QueryTimeAuditLogCleanUpHistogram = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: "jimm",
		Subsystem: "db",
		Name:      "query_audit_clean_up_duration_seconds",
		Help:      "Histogram of query time for audit_log clean up in seconds",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	})
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
	WebsocketRequestDuration = promauto.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "jimm",
		Subsystem: "websocket",
		Name:      "request_duration_seconds",
		Help:      "The duration of a websocket request in seconds.",
	}, []string{"type", "action"})
)
