// Copyright 2026 Canonical.

package river

import (
	"context"
	"fmt"
	"os"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river"
	"go.uber.org/zap"
)

// newBootstrapWorker creates a new bootstrapWorker.
func newBootstrapWorker(openfgaClient *openfga.OFGAClient, store Store, bootstrapManager BootstrapManager) (*bootstrapWorker, error) {
	if openfgaClient == nil {
		return nil, errors.E("openfgaClient is required")
	}
	if bootstrapManager == nil {
		return nil, errors.E("bootstrapManager is required")
	}
	if store == nil {
		return nil, errors.E("store is required")
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
		return errors.E(fmt.Errorf("failed to create temporary directory for Juju data: %w", err))
	}

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
