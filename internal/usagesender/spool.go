// Copyright 2017 Canonical Ltd.

package usagesender

import (
	"context"
	"sync"
	"time"

	"github.com/juju/utils"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
)

const (
	unitName  = "jimm/0"
	metricKey = "juju-model-units"
)

// MetricRecorder defines the interface used by the sendModelUsageWorker
// to record and retrieve recorded metrics.
type MetricRecorder interface {
	// AddMetric records the reported metric.
	AddMetric(ctx context.Context, modelName, model, value string, credentials []byte, created time.Time) error
	// BatchesToSend returns all recorded metric batches.
	BatchesToSend(ctx context.Context) ([]MetricBatch, error)
	// Remove removes the metric.
	Remove(uuid string) error
	// Finalize stores all unsent metrics.
	Finalize() error
}

type metricDoc struct {
	UUID        string    `bson:"_id"`
	ModelName   string    `bson:"model-name"`
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
		batches: make(map[string]metricDoc),
	}, nil
}

type metricRecorder struct {
	c       *mgo.Collection
	batches map[string]metricDoc
	m       sync.Mutex
}

// AddMetric implements the MetricsRecorder interface.
func (m *metricRecorder) AddMetric(ctx context.Context, modelName, model, value string, credentials []byte, created time.Time) error {
	uuid, err := utils.NewUUID()
	if err != nil {
		return errgo.Mask(err)
	}

	m.m.Lock()
	m.batches[uuid.String()] = metricDoc{
		UUID:        uuid.String(),
		ModelName:   modelName,
		Model:       model,
		Value:       value,
		Time:        created,
		Credentials: credentials,
	}
	m.m.Unlock()
	return nil
}

// BatchesToSend implements the MetricRecorder interface.
func (m *metricRecorder) BatchesToSend(ctx context.Context) ([]MetricBatch, error) {
	iter := m.c.Find(nil).Iter()

	batches := []MetricBatch{}
	var batch metricDoc
	for iter.Next(&batch) {
		batches = append(batches,
			MetricBatch{
				UUID:        batch.UUID,
				Created:     batch.Time,
				Credentials: batch.Credentials,
				Metrics: []Metric{{
					Key:   metricKey,
					Value: batch.Value,
					Time:  batch.Time,
					Tags: map[string]string{
						ModelTag:     batch.Model,
						ModelNameTag: batch.ModelName,
					},
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
		batches = append(batches,
			MetricBatch{
				UUID:        b.UUID,
				Created:     b.Time,
				Credentials: b.Credentials,
				Metrics: []Metric{{
					Key:   metricKey,
					Value: b.Value,
					Time:  b.Time,
					Tags: map[string]string{
						ModelTag:     b.Model,
						ModelNameTag: b.ModelName,
					},
				}},
			})
	}
	m.m.Unlock()
	return batches, nil
}

// Remove implements the MetricRecorder interface.
func (m *metricRecorder) Remove(uuid string) error {
	// first remove from the batches slice
	_, ok := m.batches[uuid]
	if ok {
		m.m.Lock()
		delete(m.batches, uuid)
		m.m.Unlock()
		return nil
	}
	// then remove from mongo
	err := m.c.RemoveId(uuid)
	if err != nil && errgo.Cause(err) != mgo.ErrNotFound {
		return errgo.Mask(err)
	}
	return nil
}

// Finalize stores all unsent metrics.
func (m *metricRecorder) Finalize() error {
	m.m.Lock()
	defer m.m.Unlock()
	// persist all batches to mongo
	for _, b := range m.batches {
		err := m.c.Insert(b)
		if err != nil && !mgo.IsDup(err) {
			return errgo.Mask(err)
		}
	}
	// clear the cache
	m.batches = make(map[string]metricDoc)
	return nil
}
