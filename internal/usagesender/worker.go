// Copyright 2017 Canonical Ltd.

// package usagesender contains the implementation of the usage sender worker,
// which reports usage information for each model in the database.
package usagesender

import (
	"fmt"
	"time"

	"github.com/juju/httprequest"
	romulus "github.com/juju/romulus/wireformat/metrics"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
)

var (
	// SenderClock holds the clock implementation used by the worker.
	SenderClock clock.Clock = clock.WallClock

	UnacknowledgedMetricBatchesCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "jem",
		Subsystem: "usagesender",
		Name:      "unacknowledged_batches",
		Help:      "The number of unacknowledged batches.",
	})

	monitorFailure = UnacknowledgedMetricBatchesCount.Set
)

func init() {
	prometheus.MustRegister(UnacknowledgedMetricBatchesCount)
}

// metricBatch extends the romulus metric batch wireformat with a model name.
// NOTE: stop-gap solution until we upgrade juju dependency.
type metricBatch struct {
	wireformat.MetricBatch

	ModelName string `json:"model-name"`
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

	response, err := w.send(batches)
	if err != nil {
		zapctx.Error(w.config.Context, "failed to send model usage", zaputil.Error(err))
		return errgo.Mask(err)
	}
	for _, userResponse := range response.UserResponses {
		for _, ackBatchUUID := range userResponse.AcknowledgedBatches {
			acknowledgedBatches[ackBatchUUID] = true
			err = recorder.Remove(w.config.Context, ackBatchUUID)
			if err != nil {
				zapctx.Warn(w.config.Context, "failed to remove recorded metric")
			}
		}
	}
	// check if all batches were acknowledged
	numberOfUnacknowledgedBatches := 0
	for _, acknowledged := range acknowledgedBatches {
		if !acknowledged {
			numberOfUnacknowledgedBatches += 1
		}
	}
	if numberOfUnacknowledgedBatches > 0 {
		zapctx.Debug(w.config.Context, "model usage receipt was not acknowledged", zap.Int("unacknowledged-batches", numberOfUnacknowledgedBatches))
		monitorFailure(float64(numberOfUnacknowledgedBatches))
	}
	return nil
}

type sendUsageRequest struct {
	httprequest.Route `httprequest:"POST"`
	Body              []metricBatch `httprequest:",body"`
}

// Send sends the given metrics to omnibus.
func (w *sendModelUsageWorker) send(usage []metricBatch) (*romulus.UserStatusResponse, error) {
	client := httprequest.Client{}
	var resp romulus.UserStatusResponse
	if err := client.CallURL(
		w.config.OmnibusURL+"/metrics",
		&sendUsageRequest{Body: usage},
		&resp,
	); err != nil {
		return nil, errgo.Mask(err)
	}
	return &resp, nil
}
