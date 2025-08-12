// Copyright 2025 Canonical.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"go.uber.org/zap"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jobtracker"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/openfga"
)

// BootstrapExecutor holds a wrapper to run a command. It is primarily for testing
// and when running the bootstrap command the implemention to be used is [DefaultBootstrapExecutor].
type BootstrapExecutor interface {
	RunWrapper(
		ctx context.Context,
		binaryPath, jujuDataDir string,
		params jujucommands.BootstrapCmdParams,
	) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error)
}

// DefaultBootstrapExecutor defines the default, and expected bootstrap executor
// to be used in a [bootstrapManager.BootstrapJob]
type DefaultBootstrapExecutor struct{}

// RunWrapper wraps the command runner and bootstrap command to be run, and then runs it for you.
// This enables the running portion of the BootstrapJob to be mocked.
func (h DefaultBootstrapExecutor) RunWrapper(
	ctx context.Context,
	binaryPath, jujuDataDir string,
	params jujucommands.BootstrapCmdParams,
) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error) {
	r := jujucommands.NewCommandRunner(binaryPath, jujuDataDir)
	command := jujucommands.NewBootstrapCmd(r)
	return command.Run(ctx, params)
}

// JobParams holds the params to run a juju bootstrap job.
type JobParams struct {
	// Runner params.

	JujuDataDir string

	// CLI Download params.

	CLIVersion string
	CLIOs      string
	CLIArch    string

	// User defined command arguments

	CloudNameAndRegion string
	ControllerName     string
	AgentVersion       string
	BootstrapTimeout   int
	CloudCred          jujucloud.CloudCredential
	// PersonalCloud is the personally defined cloud. Only necessary if the cloud is not a public
	// cloud.
	PersonalCloud jujucloud.Cloud

	// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).

	LoginTokenRefreshURL string
}

// BootstrapJob returns a [jobtracker.JobFunc] [for use in the [jobtracker.Tracker]] responsible for
// bootstrapping a controller and adding it to JIMM.
func (b *bootstrapManager) BootstrapJob(
	p JobParams,
	executor BootstrapExecutor,
	user *openfga.User,
) jobtracker.JobFunc {
	const bootstrapLockTTL = 40 * time.Minute

	return func(ctx context.Context) error {
		jobId, ok := jobtracker.JobIdFromContext(ctx)
		if !ok {
			return fmt.Errorf("failed to get job ID from context")
		}

		logCtx := zapctx.WithFields(
			ctx,
			zap.String("job-id", jobId.String()),
			zap.String("controller-name", p.ControllerName),
		)

		zapctx.Debug(
			logCtx,
			"starting bootstrap job",
		)

		if err := b.store.LockBootstrap(ctx, bootstrapLockTTL); err != nil {
			zapctx.Error(
				logCtx,
				"failed to acquire bootstrap lock",
				zap.Error(err),
			)
			return errors.E(fmt.Errorf("failed to acquire bootstrap lock: %w", err))
		}

		defer func() {
			if err := b.store.UnlockBootstrap(ctx); err != nil {
				zapctx.Error(
					logCtx,
					"failed to unlock bootstrap lock",
					zap.Error(err),
				)
			}
		}()

		err := b.store.GetController(ctx, &dbmodel.Controller{Name: p.ControllerName})
		if err == nil {
			return errors.E(errors.CodeAlreadyExists, fmt.Errorf("controller %q already exists", p.ControllerName))
		}
		if errors.ErrorCode(err) != errors.CodeNotFound {
			return errors.E(fmt.Errorf("failed to check if controller exists: %w", err))
		}

		binary, err := b.binaryStore.Get(
			ctx,
			jujuclistore.JujuBinarySpec{
				Version: p.CLIVersion,
				Os:      p.CLIVersion,
				Arch:    p.CLIArch,
			},
		)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
		}
		defer binary.Done()

		if err := b.runBootstrap(ctx, p, jobId, executor, binary, user); err != nil {
			return errors.E(fmt.Errorf("run bootstrap failed: %w", err))
		}
		return nil
	}
}

// runBootstrap wraps the logic of running a controller bootstrap for JIMM into
// a self-contained function. It is expected, and only expected to be run from within
// the [bootstrapManager.BootstrapJob].
func (b *bootstrapManager) runBootstrap(
	ctx context.Context,
	p JobParams,
	jobId uuid.UUID,
	executor BootstrapExecutor,
	binary *jujuclistore.Binary,
	user *openfga.User,
) error {
	outputCh, clientStore, cleanup, err := executor.RunWrapper(
		ctx,
		binary.FullPath,
		p.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			BootstrapTimeout:     p.BootstrapTimeout,
			LoginTokenRefreshURL: p.LoginTokenRefreshURL,
			PersonalCloud:        p.PersonalCloud,
			CloudCred:            p.CloudCred,
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to run bootstrap command: %w", err))
	}
	defer cleanup()

	for output := range outputCh {
		if output.Err != nil {
			return errors.E(fmt.Errorf("bootstrap command failed: %w", output.Err))
		}
		if writeLogErr := b.store.AddBootstrapLog(
			ctx,
			jobId,
			output.Line,
		); writeLogErr != nil {
			// If we fail to write the log, we log the error but continue.
			// This is because the bootstrap process may still succeed, and we
			// don't want to fail the entire job just because we couldn't log.
			zapctx.Error(ctx, "failed to write bootstrap log", zap.Error(writeLogErr), zap.String("jobId", jobId.String()))
		}
	}

	// We could use .CurrentController, but should bootstrap change their behaviour
	// to not set the default controller, it would break. As such we're explicitly
	// getting the controller by name.
	ctrlDetails, err := clientStore.ControllerByName(p.ControllerName)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get controller details: %w", err))
	}

	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	if err != nil {
		return errors.E(fmt.Errorf("failed to parse API endpoints for controller: %w", err))
	}
	for i := range hps {
		// Mark all the unknown scopes public.
		if hps[i].Scope == network.ScopeUnknown {
			hps[i].Scope = network.ScopePublic
		}
	}

	dbCtrl := dbmodel.Controller{
		UUID:          ctrlDetails.ControllerUUID,
		Name:          p.ControllerName,
		PublicAddress: ctrlDetails.PublicDNSName,
		CACertificate: ctrlDetails.CACert,
		Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
	}

	account, err := clientStore.AccountDetails(p.ControllerName)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get account details for controller %s: %w", p.ControllerName, err))
	}
	dbCtrlCreds := juju.ControllerCreds{
		AdminIdentityName: account.User,
		AdminPassword:     account.Password,
	}
	if err := b.jujuManager.AddController(
		ctx,
		user,
		&dbCtrl,
		dbCtrlCreds,
	); err != nil {
		return errors.E(fmt.Errorf("failed to add controller to JIMM: %w", err))
	}

	return nil
}
