// Copyright 2017 Canonical Ltd.

// UsageSender package contains the implementation of the usage sender worker,
// which reports usage information for each model in the database.
package usagesender

import (
	"fmt"
	"time"

	"github.com/juju/httprequest"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"golang.org/x/net/context"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"
)

// senderClock holds the clock implementation used by the worker.
var senderClock clock.Clock = clock.WallClock

// SendModelUsageWorkerConfig contains configuration values for the worker
// that reports model usage.
type SendModelUsageWorkerConfig struct {
	OmnibusURL string
	Pool       *jem.Pool
	Period     time.Duration
	Context    context.Context
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
		case <-senderClock.After(w.config.Period):
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
	batches := []wireformat.MetricBatch{}
	iter := j.DB.Models().Find(nil).Sort("_id").Iter()
	var model mongodoc.Model
	for iter.Next(&model) {
		unitCount, ok := model.Counts[params.UnitCount]
		if !ok {
			continue
		}
		batches = append(batches, wireformat.MetricBatch{
			UUID:        utils.MustNewUUID().String(),
			ModelUUID:   model.UUID,
			CharmUrl:    jem.OmnibusJIMMCharm,
			Created:     senderClock.Now().UTC(),
			UnitName:    "jimm/0",
			Credentials: model.UsageSenderCredentials,
			Metrics: []wireformat.Metric{{
				Key:   "juju-model-units",
				Value: fmt.Sprint(unitCount.Current),
				Time:  senderClock.Now().UTC(),
			}},
		})
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot query")
	}

	_, err := w.send(batches)
	if err != nil {
		return errgo.Mask(err)
	}
	return nil
}

type sendUsageRequest struct {
	httprequest.Route `httprequest:"POST"`
	Body              []wireformat.MetricBatch `httprequest:",body"`
}

// Send sends the given metrics to omnibus.
func (w *sendModelUsageWorker) send(usage []wireformat.MetricBatch) (*wireformat.Response, error) {
	client := httprequest.Client{}
	var resp wireformat.Response
	if err := client.CallURL(
		w.config.OmnibusURL+"/metrics",
		&sendUsageRequest{Body: usage},
		&resp,
	); err != nil {
		return nil, errgo.Mask(err)
	}
	return &resp, nil
}
