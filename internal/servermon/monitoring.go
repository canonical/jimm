// Copyright 2016 Canonical Ltd.

// The servermon package is used to update statistics used
// for monitoring the API server.
package servermon

import (
	"github.com/cloud-green/monitoring"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	AuthenticationFailCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "authentication_fail",
		Help:      "The number of failed authentications.",
	})
	AuthenticationSuccessCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "authentication_success",
		Help:      "The number of successful authentications.",
	})
	AuthenticatorPoolGet = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_get",
		Help:      "The number of times an Authenticator has been retrieved from the pool.",
	})
	AuthenticatorPoolNew = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_new",
		Help:      "The number of times a new Authenticator has been created by the pool.",
	})
	AuthenticatorPoolPut = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_put",
		Help:      "The number of times an Authenticator has been replaced into the pool.",
	})
	ConcurrentWebsocketConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "concurrent_connections",
		Help:      "The number of concurrent websocket connections",
	})
	DatabaseFailCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "database",
		Name:      "fail_count",
		Help:      "The number of times a database error was considered fatal.",
	})
	DeployedUnitCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "deployed_unit_count",
		Help:      "The number of deployed units.",
	})
	LoginFailCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "login_fail_count",
		Help:      "The number of failed logins attempted.",
	})
	LoginRedirectCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "login_redirect_count",
		Help:      "The number of logins redirected to another controller.",
	})
	LoginSuccessCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "login_success_count",
		Help:      "The number of successful logins completed.",
	})
	ModelLifetime = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "model_lifetime",
		Help:      "The length of time (in hours) models had existed at the point they are destroyed.",
		// Buckets are in hours for this histogram.
		Buckets: []float64{
			1.0 / 6,
			1.0 / 2,
			1,
			6,
			24,
			7 * 24,
			28 * 24,
			6 * 28 * 24,
			365 * 24,
			2 * 365 * 24,
			5 * 365 * 24,
		},
	})
	ModelsCreatedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "models_created_count",
		Help:      "The number of models created.",
	})
	ModelsCreatedFailCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "models_created_fail_count",
		Help:      "The number of fails attempting to create models.",
	})
	ModelsDestroyedCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "models_destroyed_count",
		Help:      "The number of models destroyed.",
	})
	MonitorDeltasReceivedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "monitor",
		Name:      "deltas_received_count",
		Help:      "The number of watcher deltas received.",
	}, []string{"controller"})
	MonitorDeltaBatchesReceivedCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "monitor",
		Name:      "delta_batches_received_count",
		Help:      "The number of watcher delta batches received.",
	}, []string{"controller"})
	MonitorErrorsCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "monitor",
		Name:      "errors_count",
		Help:      "The number of monitoring errors found.",
	}, []string{"controller"})
	MonitorLeaseGauge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "monitor",
		Name:      "lease_gauge",
		Help:      "The number of current monitor leases held",
	}, []string{"controller"})
	requestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "jem",
		Subsystem: "handler",
		Name:      "request_duration",
		Help:      "The duration of a web request in seconds.",
	}, []string{"path_pattern"})
	StatsCollectFailCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "stats_collect_fail_count",
		Help:      "The number of times we failed to collect stats from mongo.",
	})
	WebsocketRequestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "jem",
		Subsystem: "websocket",
		Name:      "request_duration",
		Help:      "The duration of a websocket request in seconds.",
	}, []string{"type", "action"})
)

func init() {
	prometheus.MustRegister(AuthenticationFailCount)
	prometheus.MustRegister(AuthenticationSuccessCount)
	prometheus.MustRegister(AuthenticatorPoolGet)
	prometheus.MustRegister(AuthenticatorPoolNew)
	prometheus.MustRegister(AuthenticatorPoolPut)
	prometheus.MustRegister(ConcurrentWebsocketConnections)
	prometheus.MustRegister(DatabaseFailCount)
	prometheus.MustRegister(DeployedUnitCount)
	prometheus.MustRegister(LoginFailCount)
	prometheus.MustRegister(LoginRedirectCount)
	prometheus.MustRegister(LoginSuccessCount)
	prometheus.MustRegister(ModelLifetime)
	prometheus.MustRegister(ModelsCreatedCount)
	prometheus.MustRegister(ModelsCreatedFailCount)
	prometheus.MustRegister(MonitorDeltasReceivedCount)
	prometheus.MustRegister(MonitorDeltaBatchesReceivedCount)
	prometheus.MustRegister(MonitorErrorsCount)
	prometheus.MustRegister(MonitorLeaseGauge)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(StatsCollectFailCount)
	prometheus.MustRegister(WebsocketRequestDuration)
	prometheus.MustRegister(monitoring.NewMgoStatsCollector("jem"))
}
