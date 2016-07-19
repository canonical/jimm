// Copyright 2016 Canonical Ltd.

package monitoring

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	requestDuration = prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Namespace: "jem",
		Subsystem: "handler",
		Name:      "request_duration",
		Help:      "The duration of a web request in seconds.",
	}, []string{"path_pattern"})
)

func init() {
	prometheus.MustRegister(requestDuration)
}
