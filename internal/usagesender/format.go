// Copyright 2018 Canonical Ltd.

package usagesender

import (
	"time"
)

const (
	ModelTag     = "model"
	ModelNameTag = "model-name"
)

// MetricBatch is a batch of metrics that will be sent to
// the metric collector
type MetricBatch struct {
	UUID        string    `json:"uuid"`
	Created     time.Time `json:"created"`
	Metrics     []Metric  `json:"metrics"`
	Credentials []byte    `json:"credentials"`
}

// Metric represents a single Metric.
type Metric struct {
	Key   string            `json:"key"`
	Value string            `json:"value"`
	Time  time.Time         `json:"time"`
	Tags  map[string]string `json:"tags"`
}

// Response represents the response from the metrics collector.
type Response struct {
	// UserStatus indicates the status of the users the metrics were sent for.
	UserStatus map[string]UserStatus `json:"user-status"`
}

// UserStatus represents the status of the user.
type UserStatus struct {
	Code string `json:"status-code"`
	Info string `json:"status-info"`
}
