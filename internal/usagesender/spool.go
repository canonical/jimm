// Copyright 2017 Canonical Ltd.

package usagesender

import (
	"sync"
	"time"

	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
)

const (
	unitName  = "jimm/0"
	metricKey = "juju-model-units"
)

// MetricRecorder defines the interface used by the sendModelUsageWorker
// to record and retrieve recorded metrics.
type MetricRecorder interface {
	// AddMetric records the reported metric.
	AddMetric(model, value string, credentials []byte, created time.Time) error
	// BatchesToSend returns all recorded metric batches.
	BatchesToSend() ([]wireformat.MetricBatch, error)
	// Remove removes the metric.
	Remove(uuid string) error
	// Finalize stores all unsent metrics.
	Finalize() error
}

type metricDoc struct {
	UUID        string    `bson:"_id"`
	Model       string    `bson:"model"`
	Value       string    `bson:"value"`
	Time        time.Time `bson:"time"`
	Credentials []byte    `bson:"credentials"`
}

var _ MetricRecorder = (*metricRecorder)(nil)

// newMetricRecorder creates a new MetricRecorder that writes and reads
// metrics to and from mongodb.
func newMetricRecorder(collection *mgo.Collection) (MetricRecorder, error) {
	if collection == nil {
		return nil, errgo.New("collection not specified")
	}
	return &metricRecorder{
		c:       collection,
		batches: make(map[string]wireformat.MetricBatch),
	}, nil
}

type metricRecorder struct {
	c       *mgo.Collection
	batches map[string]wireformat.MetricBatch
	m       sync.Mutex
}

// AddMetric implements the MetricsRecorder interface.
func (m *metricRecorder) AddMetric(model, value string, credentials []byte, created time.Time) error {
	uuid, err := utils.NewUUID()
	if err != nil {
		return errgo.Mask(err)
	}
	m.m.Lock()
	m.batches[uuid.String()] = wireformat.MetricBatch{
		UUID:        uuid.String(),
		ModelUUID:   model,
		CharmUrl:    jem.OmnibusJIMMCharm,
		UnitName:    unitName,
		Created:     created,
		Credentials: credentials,
		Metrics: []wireformat.Metric{{
			Key:   metricKey,
			Value: value,
			Time:  created,
		}},
	}
	m.m.Unlock()
	return nil
}

// BatchesToSend implements the MetricRecorder interface.
func (m *metricRecorder) BatchesToSend() ([]wireformat.MetricBatch, error) {
	iter := m.c.Find(nil).Iter()

	batches := []wireformat.MetricBatch{}
	var batch metricDoc
	for iter.Next(&batch) {
		batches = append(batches,
			wireformat.MetricBatch{
				UUID:        batch.UUID,
				ModelUUID:   batch.Model,
				CharmUrl:    jem.OmnibusJIMMCharm,
				UnitName:    unitName,
				Created:     batch.Time,
				Credentials: batch.Credentials,
				Metrics: []wireformat.Metric{{
					Key:   metricKey,
					Value: batch.Value,
					Time:  batch.Time,
				}},
			},
		)
	}
	if ierr := iter.Err(); ierr != nil {
		return nil, errgo.Mask(ierr)
	}
	if cerr := iter.Close(); cerr != nil {
		return nil, errgo.Mask(cerr)
	}
	m.m.Lock()
	for _, b := range m.batches {
		batches = append(batches, b)
	}
	m.m.Unlock()
	return batches, nil
}

// Remove implements the MetricRecorder interface.
func (m *metricRecorder) Remove(uuid string) error {
	_, ok := m.batches[uuid]
	if ok {
		m.m.Lock()
		delete(m.batches, uuid)
		m.m.Unlock()
		return nil
	}
	err := m.c.RemoveId(uuid)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// Finalize stores all unsent metrics.
func (m *metricRecorder) Finalize() error {
	m.m.Lock()
	defer m.m.Unlock()
	for _, b := range m.batches {
		if len(b.Metrics) != 1 {
			return errgo.Newf("expected 1 metric, got %d", len(b.Metrics))
		}
		err := m.c.Insert(metricDoc{
			UUID:        b.UUID,
			Model:       b.ModelUUID,
			Value:       b.Metrics[0].Value,
			Time:        b.Created,
			Credentials: b.Credentials,
		})
		if err != nil {
			return errgo.Mask(err)
		}
	}
	return nil
}
