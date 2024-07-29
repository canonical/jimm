package jimm

import (
	"context"

	"github.com/juju/zaputil/zapctx"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/servermon"
)

// UpdateMetrics updates metrics for the total numbers of controllers
// managed by JIMM as well as how many model each controller manages.
func (j *JIMM) UpdateMetrics(ctx context.Context) {
	controllerCount := 0
	j.Database.ForEachController(ctx, func(c *dbmodel.Controller) error {
		controllerCount++
		modelGauge, err := servermon.ModelCount.GetMetricWith(prometheus.Labels{"controller": c.Name})
		if err != nil {
			zapctx.Error(ctx, "failed to fetch model count gauge", zap.Error(err))
			return err
		}
		count, err := j.Database.CountModelsByController(ctx, *c)
		if err != nil {
			zapctx.Error(ctx, "failed to count models by controller", zap.Error(err), zap.String("controller", c.Name))
			return err
		}
		modelGauge.Set(float64(count))
		return nil
	})
	servermon.ControllerCount.Set(float64(controllerCount))
}
