// Copyright 2016 Canonical Ltd.

// The servermon package is used to update statistics used
// for monitoring the API server.
package servermon

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	ApplicationsRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "applications_running",
		Help:      "The current number of running applications.",
	}, []string{"ctl_path"})
	AuthenticatorPoolGet = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_get",
		Help:      "The number of times an Authenticator has been retrieved from the pool",
	})
	AuthenticatorPoolNew = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_new",
		Help:      "The number of times a new Authenticator has been created by the pool",
	})
	AuthenticatorPoolPut = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "auth",
		Name:      "pool_put",
		Help:      "The number of times an Authenticator has been replaced into the pool",
	})
	ControllersRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "controllers_running",
		Help:      "The current number of running controllers.",
	})
	DatabaseConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "database",
		Name:      "database_connections",
		Help:      "The number of database connections in use, these are shared between database sessions.",
	})
	DatabaseSessions = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "database",
		Name:      "database_sessions",
		Help:      "The number of database session objects, some of these will be sharing connections.",
	})
	MachinesRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "machines_running",
		Help:      "The current number of running machines.",
	}, []string{"ctl_path"})
	ModelsRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "models_running",
		Help:      "The current number of running models.",
	}, []string{"ctl_path"})
	requestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "jem",
		Subsystem: "handler",
		Name:      "request_duration",
		Help:      "The duration of a web request in seconds.",
	}, []string{"path_pattern"})
	UnitsRunning = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "health",
		Name:      "units_running",
		Help:      "The current number of running units.",
	}, []string{"ctl_path"})
)

func init() {
	prometheus.MustRegister(ApplicationsRunning)
	prometheus.MustRegister(AuthenticatorPoolGet)
	prometheus.MustRegister(AuthenticatorPoolNew)
	prometheus.MustRegister(AuthenticatorPoolPut)
	prometheus.MustRegister(ControllersRunning)
	prometheus.MustRegister(DatabaseConnections)
	prometheus.MustRegister(DatabaseSessions)
	prometheus.MustRegister(MachinesRunning)
	prometheus.MustRegister(ModelsRunning)
	prometheus.MustRegister(requestDuration)
	prometheus.MustRegister(UnitsRunning)
}
