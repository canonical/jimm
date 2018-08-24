// Copyright 2017 Canonical Ltd.

package usagesender

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
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
	BatchesToSend(ctx context.Context) ([]metricBatch, error)
	// Remove removes the metric.
	Remove(ctx context.Context, uuid string) error
}

type metric struct {
	ModelName   string    `json:"model-name"`
	Model       string    `json:"model"`
	Value       string    `json:"value"`
	Time        time.Time `json:"time"`
	Credentials []byte    `json:"credentials"`
}

var _ MetricRecorder = (*spoolDirMetricRecorder)(nil)

// spoolDirMetricRecorder implements the MetricsRecorder interface
// and writes metrics to a spool directory for store-and-forward.
type spoolDirMetricRecorder struct {
	spoolDir string
}

// newSpoolDirMetricRecorder creates a new MetricRecorder that writes and reads
// metrics to and from a spool directory.
func newSpoolDirMetricRecorder(spoolDir string) (MetricRecorder, error) {
	err := checkSpoolDir(spoolDir)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return &spoolDirMetricRecorder{
		spoolDir: spoolDir,
	}, nil
}

// AddMetric implements the MetricsRecorder interface.
func (m *spoolDirMetricRecorder) AddMetric(ctx context.Context, modelName, model, value string, credentials []byte, created time.Time) error {
	uuid, err := utils.NewUUID()
	if err != nil {
		return errgo.Mask(err)
	}
	dataFileName := filepath.Join(m.spoolDir, uuid.String())
	dataWriter, err := createMetricFile(dataFileName)
	if err != nil {
		return errgo.Mask(err)
	}
	mm := metric{
		ModelName:   modelName,
		Model:       model,
		Value:       value,
		Time:        created,
		Credentials: credentials,
	}
	encoder := json.NewEncoder(dataWriter)
	err = encoder.Encode(mm)
	cerr := dataWriter.Close()
	switch {
	case err != nil:
		return errgo.Mask(err)
	case cerr != nil:
		return errgo.Mask(cerr)
	default:
		return nil
	}
}

// BatchesToSend implements the MetricRecorder interface.
func (r *spoolDirMetricRecorder) BatchesToSend(ctx context.Context) ([]metricBatch, error) {
	var batches []metricBatch

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errgo.Mask(err)
		}
		if info.IsDir() {
			if path == r.spoolDir {
				return nil
			}
			return filepath.SkipDir
		}
		metrics, err := readMetrics(path)
		if err != nil {
			// if we fail to read metrics from a file, we log and continue
			zapctx.Error(ctx, "failed to read file", zaputil.Error(err))
			return nil
		}
		for _, m := range metrics {
			batches = append(batches, metricBatch{
				ModelName: m.ModelName,
				MetricBatch: wireformat.MetricBatch{
					UUID:        info.Name(),
					ModelUUID:   m.Model,
					CharmUrl:    jem.OmnibusJIMMCharm,
					UnitName:    unitName,
					Created:     m.Time,
					Credentials: m.Credentials,
					Metrics: []wireformat.Metric{{
						Key:   metricKey,
						Value: m.Value,
						Time:  m.Time,
					}},
				},
			})
		}
		return nil
	}
	if err := filepath.Walk(r.spoolDir, walker); err != nil {
		return nil, errgo.Mask(err)
	}
	return batches, nil
}

// Remove implements the MetricRecorder interface.
func (r *spoolDirMetricRecorder) Remove(_ context.Context, uuid string) error {
	dataFile := filepath.Join(r.spoolDir, uuid)
	err := os.Remove(dataFile)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

func readMetrics(file string) ([]metric, error) {
	var metrics []metric
	f, err := os.Open(file)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var m metric
		err := dec.Decode(&m)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errgo.Mask(err)
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

func createMetricFile(fileName string) (*os.File, error) {
	f, err := os.OpenFile(fileName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	return f, nil
}

func checkSpoolDir(path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return errgo.Notef(err, "failed to create spool directory: %v", path)
	}
	return nil
}

func newSliceMetricRecorder(_ string) (MetricRecorder, error) {
	return &sliceMetricRecorder{}, nil
}

var _ MetricRecorder = (*sliceMetricRecorder)(nil)

type sliceMetricRecorder struct {
	batches []metricBatch
	m       sync.Mutex
}

// AddMetric implements the MetricRecorder interface.
func (r *sliceMetricRecorder) AddMetric(_ context.Context, modelName, model, value string, credentials []byte, created time.Time) error {
	r.m.Lock()
	defer r.m.Unlock()
	uuid, err := utils.NewUUID()
	if err != nil {
		return errgo.Notef(err, "failed to create a metric uuid")
	}
	r.batches = append(r.batches, metricBatch{
		ModelName: modelName,
		MetricBatch: wireformat.MetricBatch{
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
		},
	})
	return nil
}

// BatchesToSend implements the MetricRecorder interface.
func (r *sliceMetricRecorder) BatchesToSend(_ context.Context) ([]metricBatch, error) {
	r.m.Lock()
	defer r.m.Unlock()
	return r.batches, nil
}

// Remove implement the MetricRecorder interface.
func (r *sliceMetricRecorder) Remove(_ context.Context, uuid string) error {
	// do nothing because we throw away the slice recorder on every worker
	// execution
	return nil
}
