// Copyright 2017 Canonical Ltd.

// UsageSender package contains the implementation of the usage sender worker,
// which reports usage information for each model in the database.
package usagesender

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/juju/errors"
	wireformat "github.com/juju/romulus/wireformat/metrics"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/uber-go/zap"
	"golang.org/x/net/context"

	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/internal/zapctx"
	"github.com/CanonicalLtd/jem/internal/zaputil"
	"github.com/CanonicalLtd/jem/params"
	"gopkg.in/errgo.v1"
	"gopkg.in/tomb.v2"
)

var (
	newHTTPClient = func() HTTPClient {
		return &http.Client{}
	}
)

// HTTPClient defines the http client interface for posting metrics.
type HTTPClient interface {
	Post(url string, bodyType string, body io.Reader) (*http.Response, error)
}

// senderClock holds the clock implementation used by the worker.
var senderClock clock.Clock = clock.WallClock

// SendModelUsageWorkerConfig contains configuration values for the worker
// that reports model usage.
type SendModelUsageWorkerConfig struct {
	OmnibusURL string
	JEM        *jem.JEM
	Period     time.Duration
	Context    context.Context
}

func (c *SendModelUsageWorkerConfig) validate() error {
	if c.OmnibusURL == "" {
		return errgo.New("omnibus url not specified")
	}
	if c.JEM == nil {
		return errgo.New("jem not specified")
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
	w.tomb.Go(func() error {
		return w.run()
	})
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
	defer w.config.JEM.Close()
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
	batches := []wireformat.MetricBatch{}
	iter := w.config.JEM.DB.Models().Find(nil).Sort("_id").Iter()
	var model mongodoc.Model
	for iter.Next(&model) {
		unitCount, ok := model.Counts[params.UnitCount]
		if !ok {
			zapctx.Error(w.config.Context, "failed to get current unit count for model", zap.String("model", model.Path.String()))
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
				Value: fmt.Sprintf("%d", unitCount.Current),
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

// Send sends the given metrics to omnibus.
func (w *sendModelUsageWorker) send(metrics []wireformat.MetricBatch) (*wireformat.Response, error) {
	b, err := json.Marshal(metrics)
	if err != nil {
		return nil, errors.Trace(err)
	}
	r := bytes.NewBuffer(b)
	client := newHTTPClient()
	resp, err := client.Post(w.config.OmnibusURL, "application/json", r)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to send usage data %v", resp.StatusCode)
	}

	defer resp.Body.Close()
	respReader := json.NewDecoder(resp.Body)
	metricsResponse := wireformat.Response{}
	err = respReader.Decode(&metricsResponse)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &metricsResponse, nil
}
