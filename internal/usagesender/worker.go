// Copyright 2017 Canonical Ltd.

// package usagesender contains the implementation of the usage sender worker,
// which reports usage information for each model in the database.
package usagesender

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/httprequest"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/zapctx"
	"github.com/CanonicalLtd/jimm/internal/zaputil"
	"github.com/CanonicalLtd/jimm/params"
)

var (
	// SenderClock holds the clock implementation used by the worker.
	SenderClock clock.Clock = clock.WallClock

	UnacknowledgedMetricBatchesCount = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "usagesender",
		Name:      "unacknowledged_batches",
		Help:      "The number of unacknowledged batches.",
	})
	FailuresToRemoveSentBatches = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "jem",
		Subsystem: "usagesender",
		Name:      "failures_to_remove_sent",
		Help:      "Failures to remove sent batches from local storage.",
	})

	monitorFailure             = UnacknowledgedMetricBatchesCount.Add
	monitorStoreCleanupFailure = FailuresToRemoveSentBatches.Inc
)

func init() {
	prometheus.MustRegister(UnacknowledgedMetricBatchesCount)
	prometheus.MustRegister(FailuresToRemoveSentBatches)
}

// SendModelUsageWorkerConfig contains configuration values for the worker
// that reports model usage.
type SendModelUsageWorkerConfig struct {
	OmnibusURL     string
	Pool           *jem.Pool
	Period         time.Duration
	Context        context.Context
	SpoolDirectory string
}

func (c *SendModelUsageWorkerConfig) validate() error {
	if c.OmnibusURL == "" {
		return errgo.New("omnibus url not specified")
	}
	if c.Pool == nil {
		return errgo.New("pool not specified")
	}
	if c.Period == 0 {
		return errgo.New("period not specified")
	}
	if c.Context == nil {
		return errgo.New("context not specified")
	}
	return nil
}

// NewSendModelUsageWorker starts and returns a new worker that reports model usage.
func NewSendModelUsageWorker(config SendModelUsageWorkerConfig) (*sendModelUsageWorker, error) {
	if err := config.validate(); err != nil {
		return nil, errgo.Mask(err)
	}
	w := &sendModelUsageWorker{
		config: config,
	}
	w.tomb.Go(w.run)
	return w, nil
}

type sendModelUsageWorker struct {
	config SendModelUsageWorkerConfig
	tomb   tomb.Tomb
}

// Kill implements worker.Worker.Kill.
func (w *sendModelUsageWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (w *sendModelUsageWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *sendModelUsageWorker) run() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-SenderClock.After(w.config.Period):
			err := w.execute()
			if err != nil {
				zapctx.Error(w.config.Context, "failed to send usage information", zaputil.Error(err))
			}
		}
	}
}

func (w *sendModelUsageWorker) execute() error {
	j := w.config.Pool.JEM(w.config.Context)
	defer j.Close()
	acknowledgedBatches := make(map[string]bool)
	iter := j.DB.Models().Find(nil).Sort("_id").Iter()

	recorder, err := newSliceMetricRecorder(w.config.SpoolDirectory)
	if err != nil {
		zapctx.Error(w.config.Context, "failed to create a metric recorder", zaputil.Error(err))
		return errgo.Mask(err)
	}
	if w.config.SpoolDirectory != "" {
		recorder, err = newSpoolDirMetricRecorder(w.config.SpoolDirectory)
		if err != nil {
			zapctx.Error(w.config.Context, "failed to create a metric recorder", zaputil.Error(err))
			return errgo.Notef(err, "failed to create a metric recorder")
		}

	}

	var model mongodoc.Model
	for iter.Next(&model) {
		unitCount, ok := model.Counts[params.UnitCount]
		if !ok {
			zapctx.Debug(w.config.Context, "model unit count not found", zap.String("model-uuid", model.UUID))
			continue
		}
		t := SenderClock.Now().UTC()
		err = recorder.AddMetric(w.config.Context, string(model.Path.String()), model.UUID, fmt.Sprintf("%d", unitCount.Current), model.UsageSenderCredentials, t)
		if err != nil {
			zapctx.Error(w.config.Context, "failed to record a metric", zaputil.Error(err))
			continue
		}
	}
	if err := iter.Err(); err != nil {
		zapctx.Error(w.config.Context, "model query failed", zaputil.Error(err))
		return errgo.Notef(err, "model query failed")
	}

	batches, err := recorder.BatchesToSend(w.config.Context)
	if err != nil {
		zapctx.Error(w.config.Context, "failed to read recorded metrics", zaputil.Error(err))
		return errgo.Notef(err, "failed to read recorded metrics")
	}
	for _, b := range batches {
		acknowledgedBatches[b.UUID] = false
	}

	_, err = w.send(batches)
	if err != nil {
		zapctx.Error(w.config.Context, "failed to send model usage", zaputil.Error(err))
		monitorFailure(float64(len(batches)))
		return errgo.Mask(err)
	}
	for _, batch := range batches {
		err = recorder.Remove(w.config.Context, batch.UUID)
		if err != nil {
			monitorStoreCleanupFailure()
			zapctx.Warn(w.config.Context, "failed to remove recorded metric")
		}
	}
	return nil
}

type sendUsageRequest struct {
	httprequest.Route `httprequest:"POST"`
	Body              []MetricBatch `httprequest:",body"`
}

// Send sends the given metrics to omnibus.
func (w *sendModelUsageWorker) send(usage []MetricBatch) (*Response, error) {
	client := httprequest.Client{}
	var resp Response
	if err := client.CallURL(
		w.config.OmnibusURL+"/v4/jimm/metrics",
		&sendUsageRequest{Body: usage},
		&resp,
	); err != nil {
		return nil, errgo.Mask(err)
	}
	return &resp, nil
}
