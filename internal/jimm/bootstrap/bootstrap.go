// Copyright 2025 Canonical.

// bootstrap package provides functionality to manage the bootstrap process
// for controllers in JIMM.
package bootstrap

import (
	"context"
	"fmt"
	"os"
	"runtime"
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
	"github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	binaryDone = func(b *jujuclistore.Binary) {
		b.Done()
	}
)

const (
	bootstrapJobType     = "bootstrap"
	maxBootstrapDuration = 60 * time.Minute
)

// Store defines the store methods required by the manager.
type Store interface {
	QueryBootstrapLog(ctx context.Context, jobId uuid.UUID, offset int) (loggies []string, nextOffsetValue int, err error)

	// BootstrapJob store methods:

	LockBootstrap(ctx context.Context, ttl time.Duration) error
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
	AddBootstrapLog(ctx context.Context, jobId uuid.UUID, logLine string) (err error)
	UnlockBootstrap(ctx context.Context) error
}

// JobTracker interface defines the methods required for job tracking.
type JobTracker interface {
	// GetJob retrieves a job entry by its ID.
	GetJob(ctx context.Context, jobId uuid.UUID) (dbmodel.JobTrackerEntry, error)
	// StopJob stops a job by its ID.
	StopJob(ctx context.Context, jobId uuid.UUID) error
	// Run runs a new job and returns the job ID.
	Run(ctx context.Context, jobType string, job jobtracker.JobFunc, maxDuration time.Duration) (uuid.UUID, error)
}

// JujuManager defines the juju manager methods required by the job.
type JujuManager interface {
	AddController(ctx context.Context, user *openfga.User, ctl *dbmodel.Controller, creds juju.ControllerCreds) error
}

// BinaryStore defines the binary store methods required by the job.
type BinaryStore interface {
	Get(ctx context.Context, spec jujuclistore.JujuBinarySpec) (*jujuclistore.Binary, error)
}

type bootstrapManager struct {
	store                     Store
	tracker                   JobTracker
	jujuManager               JujuManager
	binaryStore               BinaryStore
	jimmWellknownJWKSEndpoint string
}

// NewBootstrapManager creates a new BootstrapManager instance.
func NewBootstrapManager(
	store Store,
	jobtracker JobTracker,
	jujuManager JujuManager,
	binaryStore BinaryStore,
	jimmWellknownJWKSEndpoint string,
) (*bootstrapManager, error) {
	if store == nil {
		return nil, errors.E("store cannot be nil")
	}
	if jobtracker == nil {
		return nil, errors.E("job tracker cannot be nil")
	}
	if jujuManager == nil {
		return nil, errors.E("juju manager cannot be nil")
	}
	if binaryStore == nil {
		return nil, errors.E("binary store cannot be nil")
	}
	if jimmWellknownJWKSEndpoint == "" {
		return nil, errors.E("jimm well-known JWKs endpoint cannot be empty")
	}
	return &bootstrapManager{
		store:                     store,
		tracker:                   jobtracker,
		jujuManager:               jujuManager,
		binaryStore:               binaryStore,
		jimmWellknownJWKSEndpoint: jimmWellknownJWKSEndpoint,
	}, nil
}

// GetBootstrapStatusAndLogs retrieves the status and logs of a bootstrap job.
// It requires the user to be an admin and returns the status, error message, logs,
// and a watermark for pagination.
func (b *bootstrapManager) GetBootstrapStatusAndLogs(ctx context.Context, _ *openfga.User, jobId uuid.UUID, offset int) (params.BootstrapStatusResponse, error) {
	const op = errors.Op("jimm.GetBootstrapStatusAndLogs")

	job, err := b.tracker.GetJob(ctx, jobId)
	if err != nil {
		return params.BootstrapStatusResponse{}, errors.E(op, "failed to get job status", err)
	}

	loggies, newOffset, err := b.store.QueryBootstrapLog(ctx, jobId, offset)
	if err != nil {
		return params.BootstrapStatusResponse{}, errors.E(op, "failed to query bootstrap logs", err)
	}
	return params.BootstrapStatusResponse{
		Status:    params.JobStatus(job.Status),
		Error:     job.Error,
		Logs:      loggies,
		Watermark: newOffset,
	}, nil
}

// StopBootstrap stops a bootstrap job by its ID.
func (b *bootstrapManager) StopBootstrap(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
	const op = errors.Op("jimm.StopBootstrap")

	if user == nil {
		return errors.E(op, "user cannot be nil")
	}

	if jobId == uuid.Nil {
		return errors.E(op, "job ID cannot be nil")
	}

	err := b.tracker.StopJob(ctx, jobId)
	if err != nil {
		return errors.E(op, "failed to stop job", err)
	}

	return nil
}

// StartBootstrap starts a bootstrap job with the provided parameters.
func (b *bootstrapManager) StartBootstrap(ctx context.Context, user *openfga.User, params BootstrapParams) (string, error) {
	const op = errors.Op("jimm.StartBootstrap")

	if err := params.validate(); err != nil {
		return "", errors.E(op, fmt.Errorf("invalid bootstrap parameters: %v", err))
	}

	temp, err := os.MkdirTemp("", "juju-data-dir")
	if err != nil {
		return "", errors.E(op, fmt.Errorf("failed to create temporary directory for Juju data: %w", err))
	}

	jobId, err := b.tracker.Run(
		ctx,
		bootstrapJobType,
		b.BootstrapJob(
			JobParams{
				// Runner args.
				JujuDataDir: temp,
				// Binary args.
				CLIVersion: params.CLIVersion,
				// User defined command arguments
				CloudNameAndRegion: params.CloudNameAndRegion,
				ControllerName:     params.ControllerName,
				BootstrapTimeout:   params.BootstrapTimeout,
				CloudCred:          params.CloudCred,
				PersonalCloud:      params.PersonalCloud,
				// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).
				LoginTokenRefreshURL: b.jimmWellknownJWKSEndpoint,

				PublicDNSAddress:       params.PublicDNSAddress,
				ControllerServiceType:  params.ControllerServiceType,
				ControllerExternalIPs:  params.ControllerExternalIPs,
				ControllerExternalName: params.ControllerExternalName,
			},
			DefaultBootstrapExecutor{},
			user,
		),
		maxBootstrapDuration,
	)
	if err != nil {
		return "", errors.E(op, fmt.Errorf("failed to start bootstrap job: %w", err))
	}

	return jobId.String(), nil
}

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

	// Controller public dns address (if any) and k8s service options to expose a k8s
	// controller.

	PublicDNSAddress       string
	ControllerServiceType  string
	ControllerExternalIPs  string
	ControllerExternalName string
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

		// TODO: If we remove the 1 bootstrap lock, in theory two API requests to bootstrap controllers
		// could be made at the same time, and both would pass this check but only one could succeed.
		// This needs to be fixed.
		err := b.store.GetController(ctx, &dbmodel.Controller{Name: p.ControllerName})
		if err == nil {
			return errors.E(errors.CodeAlreadyExists, fmt.Errorf("controller %q already exists", p.ControllerName))
		}
		if errors.ErrorCode(err) != errors.CodeNotFound {
			return errors.E(fmt.Errorf("failed to check if controller exists: %w", err))
		}

		// TODO(ale8k): When downloading, it takes a while and the CLI has no output. Download progress would be a VERY nice to have here.
		binary, err := b.binaryStore.Get(
			ctx,
			jujuclistore.JujuBinarySpec{
				Version: p.CLIVersion,
				// This is a best effort. The launchpad URL just so happens to have similar filenames to runtime.GOOS and runtime.GOARCH.
				// If this ever changes, we should update the binary store to use the correct URL
				// or we should detect the OS and arch here. We don't want to detect it in the binary store
				// because the consumer may want to detect it in different ways.
				Os:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
		)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
		}
		zapctx.Debug(logCtx, "Juju binary downloaded, using Juju binary", zap.String("binary-path", binary.FullPath))
		defer binaryDone(binary)

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
			CloudNameAndRegion:     p.CloudNameAndRegion,
			ControllerName:         p.ControllerName,
			AgentVersion:           p.AgentVersion,
			BootstrapTimeout:       p.BootstrapTimeout,
			LoginTokenRefreshURL:   p.LoginTokenRefreshURL,
			PersonalCloud:          p.PersonalCloud,
			CloudCred:              p.CloudCred,
			PublicDNSAddress:       p.PublicDNSAddress,
			ControllerServiceType:  p.ControllerServiceType,
			ControllerExternalIPs:  p.ControllerExternalIPs,
			ControllerExternalName: p.ControllerExternalName,
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to run bootstrap command: %w", err))
	}
	defer cleanup()

	for output := range outputCh {
		if output.Err != nil {
			if writeLogErr := b.store.AddBootstrapLog(
				ctx,
				jobId,
				output.Err.Error(),
			); writeLogErr != nil {
				zapctx.Error(ctx, "failed to write bootstrap error", zap.Error(writeLogErr), zap.String("jobId", jobId.String()))
			}
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
	// TODO(ale8k): At the moment, we can't reach added k8s controllers because the controller is not reachable.
	// Card: https://warthogs.atlassian.net/browse/JUJU-8436

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
		TLSHostname:   "juju-apiserver",
	}

	account, err := clientStore.AccountDetails(p.ControllerName)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get account details for controller %s: %w", p.ControllerName, err))
	}
	dbCtrlCreds := juju.ControllerCreds{
		AdminIdentityName: account.User,
		AdminPassword:     account.Password,
	}
	// TODO(ale8k): Fix this...
	// If the controller cannot be reached, we'll end up with dangling resources to a now no-longer accessible controller (from the clients perspective).
	// So... Find a nice way to remove controllers if the add fail / clean up dangling ones later.
	// Once fixed, this can be tested by just making the controller addr something that doesn't exist, and we can test the solution.
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
