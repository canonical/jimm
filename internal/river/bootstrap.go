// Copyright 2026 Canonical.

package river

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

// newBootstrapWorker creates a new bootstrapWorker.
func newBootstrapWorker(openfgaClient *openfga.OFGAClient, store Store, bootstrapManager BootstrapManager) (*bootstrapWorker, error) {
	if openfgaClient == nil {
		return nil, errors.New("openfgaClient is required")
	}
	if bootstrapManager == nil {
		return nil, errors.New("bootstrapManager is required")
	}
	if store == nil {
		return nil, errors.New("store is required")
	}

	return &bootstrapWorker{
		openfgaClient:    openfgaClient,
		bootstrapManager: bootstrapManager,
		store:            store,
	}, nil
}

type bootstrapWorker struct {
	// An embedded WorkerDefaults sets up default methods to fulfill the rest of
	// the Worker interface:
	river.WorkerDefaults[rivertypes.BootstrapArgs]

	openfgaClient    *openfga.OFGAClient
	store            Store
	bootstrapManager BootstrapManager
}

// Work implements the [river.Worker] interface.
func (w *bootstrapWorker) Work(ctx context.Context, job *river.Job[rivertypes.BootstrapArgs]) error {
	ctx = zapctx.WithFields(ctx,
		zap.String("controller-name", job.Args.ControllerName),
		zap.Int64("job-id", job.ID),
	)

	zapctx.Debug(ctx, "starting bootstrap-controller job")

	u := &dbmodel.Identity{Name: job.Args.Username}
	if err := w.store.FetchIdentity(ctx, u); err != nil {
		return err
	}
	user := openfga.NewUser(u, w.openfgaClient)

	temp, err := os.MkdirTemp("", "juju-data-dir")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory for Juju data: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(temp); err != nil {
			zapctx.Error(ctx, "failed to remove temporary Juju data directory", zap.String("path", temp), zap.Error(err))
		}
	}()

	bootstrapArgs := bootstrap.RunBootstrapArgs{
		BootstrapArgs: job.Args,
		RunnerArgs: bootstrap.RunnerArgs{
			JujuDataDir: temp,
			JobID:       job.ID,
		},
	}

	if err := w.bootstrapManager.BootstrapController(ctx, bootstrapArgs, &bootstrap.JujuCLI{}, user); err != nil {
		return err
	}

	return nil
}

// Timeout implements the [river.Worker] interface.
// Bootstrap operations can take a while to complete so we set a generous timeout.
func (w *bootstrapWorker) Timeout(*river.Job[rivertypes.BootstrapArgs]) time.Duration {
	return 60 * time.Minute
}
