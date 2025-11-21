// Copyright 2025 Canonical.

// bootstrap package provides functionality to manage the bootstrap process
// for controllers in JIMM.
package bootstrap

import (
	"context"
	"fmt"
	"net/url"
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
	"gopkg.in/yaml.v3"

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
	bootstrapJobType         = "bootstrap"
	destroyControllerJobType = "destroy-controller"
	maxJobDuration           = 60 * time.Minute
)

// Store defines the store methods required by the manager.
type Store interface {
	QueryJobLog(ctx context.Context, jobId uuid.UUID, offset int) (loggies []string, nextOffsetValue int, err error)

	// BootstrapJob store methods:

	LockBootstrap(ctx context.Context, ttl time.Duration) error
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
	AddJobLog(ctx context.Context, jobId uuid.UUID, logLine string) (err error)
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
	RemoveController(ctx context.Context, user *openfga.User, controllerName string, force bool) error
}

// BinaryStore defines the binary store methods required by the job.
type BinaryStore interface {
	Get(ctx context.Context, spec jujuclistore.JujuBinarySpec, logFunction func(string)) (*jujuclistore.Binary, error)
}

// JujuCommands defines the Juju CLI methods that the bootstrap-related jobs require.
type JujuCommands interface {
	Bootstrap(ctx context.Context, p jujucommands.BootstrapCmdParams) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error)
	DestroyController(ctx context.Context, p jujucommands.DestroyControllerCmdParams) (<-chan jujucommands.OutputLine, error)
}

// CredentialStore lets us fetch credentials from Vault
type CredentialStore interface {
	GetControllerCredentials(ctx context.Context, controllerName string) (string, string, error)
}

// CommandFactory is a wrapper for mocking Juju commands, with a concrete
// implementation in [commandFactory].
type CommandFactory interface {
	New(binaryPath, jujuDataDir string) JujuCommands
}

type bootstrapManager struct {
	store                     Store
	tracker                   JobTracker
	jujuManager               JujuManager
	binaryStore               BinaryStore
	jimmWellknownJWKSEndpoint string
	credentialStore           CredentialStore
}

// NewBootstrapManager creates a new BootstrapManager instance.
func NewBootstrapManager(
	store Store,
	jobtracker JobTracker,
	jujuManager JujuManager,
	binaryStore BinaryStore,
	jimmWellknownJWKSEndpoint string,
	credentialStore CredentialStore,
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
	// validate the JWKs endpoint URL if provided.
	if jimmWellknownJWKSEndpoint != "" {
		// Scheme is not optional, so we aren't using ParseURLWithOptionalScheme here.
		if _, err := url.Parse(jimmWellknownJWKSEndpoint); err != nil {
			return nil, errors.E(err, "failed to parse bootstrap login token refresh URL")
		}
	}
	if credentialStore == nil {
		return nil, errors.E("credential store cannot be nil")
	}
	return &bootstrapManager{
		store:                     store,
		tracker:                   jobtracker,
		jujuManager:               jujuManager,
		binaryStore:               binaryStore,
		jimmWellknownJWKSEndpoint: jimmWellknownJWKSEndpoint,
		credentialStore:           credentialStore,
	}, nil
}

// GetJobInfo retrieves the status and logs of a bootstrap job.
// It requires the user to be an admin and returns the status, error message, logs,
// and a watermark for pagination.
func (b *bootstrapManager) GetJobInfo(ctx context.Context, _ *openfga.User, jobId uuid.UUID, offset int) (params.GetJobInfoResponse, error) {
	const op = errors.Op("jimm.GetJobInfo")

	job, err := b.tracker.GetJob(ctx, jobId)
	if err != nil {
		return params.GetJobInfoResponse{}, errors.E(op, "failed to get job info", err)
	}

	loggies, newOffset, err := b.store.QueryJobLog(ctx, jobId, offset)
	if err != nil {
		return params.GetJobInfoResponse{}, errors.E(op, "failed to query job logs", err)
	}
	return params.GetJobInfoResponse{
		Status:    params.JobStatus(job.Status),
		Error:     job.Error,
		Logs:      loggies,
		Watermark: newOffset,
	}, nil
}

// StopJob stops a bootstrap job by its ID.
func (b *bootstrapManager) StopJob(ctx context.Context, user *openfga.User, jobId uuid.UUID) error {
	const op = errors.Op("jimm.StopJob")

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
func (b *bootstrapManager) StartBootstrapJob(ctx context.Context, user *openfga.User, params BootstrapParams) (string, error) {
	const op = errors.Op("jimm.StartBootstrapJob")

	if b.jimmWellknownJWKSEndpoint == "" {
		return "", errors.E(op, "bootstrap login token refresh URL is not configured. Cannot proceed with bootstrap. Please configure it and try again.")
	}

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
				CloudCred:          params.CloudCred,
				PersonalCloud:      params.PersonalCloud,
				// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).
				LoginTokenRefreshURL: b.jimmWellknownJWKSEndpoint,
				// User defined config
				UserConfig: params.UserConfig,
			},
			commandFactory{},
			user,
		),
		maxJobDuration,
	)
	if err != nil {
		return "", errors.E(op, fmt.Errorf("failed to start bootstrap job: %w", err))
	}

	return jobId.String(), nil
}

type command struct {
	binaryPath  string
	jujuDataDir string
}

// Bootstrap implements the JujuCommands interface.
func (c command) Bootstrap(ctx context.Context, p jujucommands.BootstrapCmdParams) (
	<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error) {

	r := jujucommands.NewCommandRunner(c.binaryPath, c.jujuDataDir)
	return jujucommands.NewBootstrapCmd(r).Run(ctx, p)
}

// DestroyController implements the JujuCommands interface.
func (c command) DestroyController(ctx context.Context, p jujucommands.DestroyControllerCmdParams) (
	<-chan jujucommands.OutputLine, error) {

	r := jujucommands.NewCommandRunner(c.binaryPath, c.jujuDataDir)
	return jujucommands.NewDestroyControllerCmd(r).Run(ctx, p)
}

// commandFactory provides a concrete implementation of [CommandFactory]
// to be used in a [bootstrapManager.BootstrapJob]
type commandFactory struct{}

// New create a new JujuCommands implementation.
func (h commandFactory) New(binaryPath, jujuDataDir string) JujuCommands {
	return command{
		binaryPath:  binaryPath,
		jujuDataDir: jujuDataDir,
	}
}

// RunWrapper wraps the command runner and bootstrap command to be run, and then runs it for you.
// This enables the running portion of the BootstrapJob to be mocked.
func (h commandFactory) RunWrapper(
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
	CloudCred          jujucloud.Credential
	// PersonalCloud is the personally defined cloud. Only necessary if the cloud is not a public
	// cloud.
	PersonalCloud jujucloud.Cloud

	// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).

	LoginTokenRefreshURL string

	// User provided config
	UserConfig map[string]string
}

// BootstrapJob returns a [jobtracker.JobFunc] [for use in the [jobtracker.Tracker]] responsible for
// bootstrapping a controller and adding it to JIMM.
func (b *bootstrapManager) BootstrapJob(
	p JobParams,
	cmdFactory CommandFactory,
	user *openfga.User,
) jobtracker.JobFunc {
	return func(jobCtx context.Context) error {
		jobId, ok := jobtracker.JobIdFromContext(jobCtx)
		if !ok {
			return fmt.Errorf("failed to get job ID from context")
		}

		jobCtx = zapctx.WithFields(
			jobCtx,
			zap.String("job-id", jobId.String()),
			zap.String("controller-name", p.ControllerName),
		)

		zapctx.Debug(
			jobCtx,
			"starting bootstrap job",
		)

		// Lock the bootstrap for the same length the process is allowed to run for
		// before being killed.
		if err := b.store.LockBootstrap(jobCtx, jujucommands.CommandKillDelay); err != nil {
			return errors.E(fmt.Errorf("failed to acquire bootstrap lock: %w", err))
		}

		// Use a background context to unlock the bootstrap lock.
		// This ensures that the lock is released even if the job context is cancelled.
		defer func() {
			if err := b.store.UnlockBootstrap(context.Background()); err != nil {
				zapctx.Error(
					jobCtx,
					"failed to unlock bootstrap lock",
					zap.Error(err),
				)
			}
		}()

		// TODO: If we remove the 1 bootstrap lock, in theory two API requests to bootstrap controllers
		// could be made at the same time, and both would pass this check but only one could succeed.
		// This needs to be fixed.
		err := b.store.GetController(jobCtx, &dbmodel.Controller{Name: p.ControllerName})
		if err == nil {
			return errors.E(errors.CodeAlreadyExists, fmt.Errorf("controller %q already exists", p.ControllerName))
		}
		if errors.ErrorCode(err) != errors.CodeNotFound {
			return errors.E(fmt.Errorf("failed to check if controller exists: %w", err))
		}

		b.writeJobLog(jobCtx, jobId,
			fmt.Sprintf("Downloading the Juju CLI, version %s for bootstrap. This may take a few minutes", p.CLIVersion))

		binary, err := b.binaryStore.Get(
			jobCtx,
			jujuclistore.JujuBinarySpec{
				Version: p.CLIVersion,
				Os:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			},
			func(line string) {
				b.writeJobLog(jobCtx, jobId, line)
			},
		)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
		}
		zapctx.Debug(jobCtx, "Juju binary downloaded, using Juju binary", zap.String("binary-path", binary.FullPath))
		defer binaryDone(binary)

		jujuCmds := cmdFactory.New(binary.FullPath, p.JujuDataDir)

		if err := b.runBootstrap(jobCtx, p, jobId, jujuCmds, user); err != nil {
			return errors.E(fmt.Errorf("run bootstrap failed: %w", err))
		}
		return nil
	}
}

// runBootstrap wraps the logic of running a controller bootstrap for JIMM into
// a self-contained function. It is expected, and only expected to be run from within
// the [bootstrapManager.BootstrapJob].
//
// The jobCtx is expected to be the context of the job, and as such will be cancelled
// when the job is stopped or cancelled. The jobCtx is NOT expected to be used for
// any store operations, or other operations that should continue even if the job
// is cancelled.
func (b *bootstrapManager) runBootstrap(
	jobCtx context.Context,
	p JobParams,
	jobId uuid.UUID,
	executor JujuCommands,
	user *openfga.User,
) error {

	outputCh, clientStore, cleanup, err := executor.Bootstrap(
		jobCtx,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			LoginTokenRefreshURL: p.LoginTokenRefreshURL,
			PersonalCloud:        p.PersonalCloud,
			CloudCred:            p.CloudCred,
			UserConfig:           p.UserConfig,
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to run bootstrap command: %w", err))
	}
	defer cleanup()

	// Update the context from this point to prevent it from being cancelled when the parent is cancelled.
	// This ensures that we still capture output from the bootstrap command
	// and log it if that command is cancelled while keeping things like
	// log info that was set on the context.
	jobCtx = context.WithoutCancel(jobCtx)

	err = b.consumeCommandOutput(jobCtx, outputCh, jobId)
	if err != nil {
		return err
	}

	// controllerCleanup is a helper function to cleanup the controller if we fail at any point
	// after the bootstrap, avoiding orphaned controllers in the cloud.
	controllerCleanup := func(err error, controllerDetails *jujuclient.ControllerDetails) error {
		cleanupErr := b.tryCleanupController(jobCtx, executor, jobId, p.ControllerName)
		if cleanupErr == nil {
			return errors.E(fmt.Errorf("error post-bootstrap: %w\n"+
				"the controller has been automatically destroyed", err))
		}
		var controllerDetailsStr string
		if controllerDetails != nil {
			res, _ := yaml.Marshal(controllerDetails)
			controllerDetailsStr = string(res)
		}

		zapctx.Error(jobCtx, "failed to cleanup controller after failing to add it to JIMM",
			zap.NamedError("BootstrapError", err), zap.NamedError("CleanupError", cleanupErr))
		return errors.E(fmt.Errorf("error post-bootstrap: %w\n"+
			"automatic cleanup of the controller also failed: %w\n"+
			"\n"+
			"WARNING: resources associated with the controller may remain dangling in your environment.\n"+
			"Manual intervention is required, either attach the controller to JIMM or destroy it.\n"+
			"\n"+
			"Controller details:\n%s", err, cleanupErr, controllerDetailsStr))

	}

	// We could use .CurrentController, but should bootstrap change their behaviour
	// to not set the default controller, it would break. As such we're explicitly
	// getting the controller by name.
	ctrlDetails, err := clientStore.ControllerByName(p.ControllerName)
	if err != nil {
		return controllerCleanup(fmt.Errorf("failed to get controller details: %w", err), nil)
	}

	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	if err != nil {
		return controllerCleanup(fmt.Errorf("failed to parse API endpoints for controller: %w", err), ctrlDetails)
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
		return controllerCleanup(fmt.Errorf("failed to get account details for controller %s: %w", p.ControllerName, err), ctrlDetails)
	}
	dbCtrlCreds := juju.ControllerCreds{
		AdminIdentityName: account.User,
		AdminPassword:     account.Password,
	}
	if err := b.jujuManager.AddController(
		jobCtx,
		user,
		&dbCtrl,
		dbCtrlCreds,
	); err != nil {
		return controllerCleanup(fmt.Errorf("failed to add controller to JIMM: %w", err), ctrlDetails)
	}

	return nil
}

func (b *bootstrapManager) tryCleanupController(ctx context.Context, jujuCmd JujuCommands, jobID uuid.UUID, controllerName string) error {
	outputCh, err := jujuCmd.DestroyController(
		ctx,
		jujucommands.DestroyControllerCmdParams{
			ControllerName: controllerName,
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to run destroy-controller command: %w", err))
	}

	err = b.consumeCommandOutput(ctx, outputCh, jobID)
	if err != nil {
		return err
	}

	return nil
}

func (b *bootstrapManager) consumeCommandOutput(ctx context.Context, outputCh <-chan jujucommands.OutputLine, jobId uuid.UUID) error {
	for output := range outputCh {
		if output.Err != nil {
			b.writeJobLog(ctx, jobId, output.Err.Error())
			return errors.E(fmt.Errorf("command failed: %w", output.Err))
		}
		b.writeJobLog(ctx, jobId, output.Line)
	}
	return nil
}

// writeJobLog writes logs to the store to eventually be displayed to users.
// Errors are masked but logged to avoid failing the bootstrap process.
func (b *bootstrapManager) writeJobLog(ctx context.Context, jobId uuid.UUID, logLine string) {
	if err := b.store.AddJobLog(ctx, jobId, logLine); err != nil {
		zapctx.Error(ctx, "failed to write bootstrap log", zap.Error(err), zap.String("jobId", jobId.String()))
	}
}

// StartDestroyControllerJob starts a destroy-controller job
func (b *bootstrapManager) StartDestroyControllerJob(ctx context.Context, user *openfga.User, params DestroyControllerParams) (string, error) {
	const op = errors.Op("jimm.StartDestroyControllerJob")

	jujuDataDir, err := os.MkdirTemp("", "juju-data-dir")
	if err != nil {
		return "", errors.E(op, fmt.Errorf("failed to create temporary directory for Juju data: %w", err))
	}

	jobId, err := b.tracker.Run(
		ctx,
		destroyControllerJobType,
		b.DestroyControllerJob(params, commandFactory{}, user, jujuDataDir),
		maxJobDuration,
	)
	if err != nil {
		return "", errors.E(op, fmt.Errorf("failed to start bootstrap job: %w", err))
	}

	return jobId.String(), nil
}

func (b *bootstrapManager) DestroyControllerJob(
	params DestroyControllerParams,
	cmdFactory CommandFactory,
	user *openfga.User,
	jujuDataDir string,
) jobtracker.JobFunc {
	return func(jobCtx context.Context) error {
		jobId, ok := jobtracker.JobIdFromContext(jobCtx)
		if !ok {
			return fmt.Errorf("failed to get job ID from context")
		}

		jobCtx = zapctx.WithFields(
			jobCtx,
			zap.String("job-id", jobId.String()),
			zap.String("controller-name", params.ControllerName),
		)

		zapctx.Debug(
			jobCtx,
			"starting destroy-controller job",
		)

		// Lock the bootstrap for the same length the process is allowed to run for
		// before being killed.
		if err := b.store.LockBootstrap(jobCtx, jujucommands.CommandKillDelay); err != nil {
			return errors.E(fmt.Errorf("failed to acquire bootstrap lock: %w", err))
		}

		// Use a background context to unlock the bootstrap lock.
		// This ensures that the lock is released even if the job context is cancelled.
		defer func() {
			if err := b.store.UnlockBootstrap(context.Background()); err != nil {
				zapctx.Error(
					jobCtx,
					"failed to unlock bootstrap lock",
					zap.Error(err),
				)
			}
		}()

		b.writeJobLog(jobCtx, jobId,
			fmt.Sprintf("Downloading the Juju CLI, version %s for destroy-controller. This may take a few minutes", params.AgentVersion))

		binary, err := b.binaryStore.Get(
			jobCtx,
			jujuclistore.JujuBinarySpec{
				Version: params.AgentVersion,
				Os:      runtime.GOOS,
				Arch:    runtime.GOARCH,
			},
			func(line string) {
				b.writeJobLog(jobCtx, jobId, line)
			},
		)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
		}
		zapctx.Debug(jobCtx, "Juju binary downloaded, using Juju binary", zap.String("binary-path", binary.FullPath))
		defer func() {
			binary.Done()
		}()

		jujuCmd := cmdFactory.New(binary.FullPath, jujuDataDir)

		username, password, err := b.credentialStore.GetControllerCredentials(jobCtx, params.ControllerName)
		if err != nil {
			return errors.E(fmt.Errorf("failed to get controller credentials: %w", err))
		}

		// Update the context from this point to prevent it from being cancelled when the parent is cancelled.
		// This ensures that we still capture output from the destroy-controller command
		// and log it if that command is cancelled while keeping things like
		// log info that was set on the context.
		jobCtx = context.WithoutCancel(jobCtx)

		outputCh, err := jujuCmd.DestroyController(
			jobCtx,
			jujucommands.DestroyControllerCmdParams{
				ControllerName: params.ControllerName,
				ControllerDetails: jujuclient.ControllerDetails{
					ControllerUUID: params.ControllerUUID,
					Cloud:          params.CloudName,
					CloudRegion:    params.CloudRegion,
					APIEndpoints:   params.APIEndpoints,
					CACert:         params.CACertificate,
					PublicDNSName:  params.PublicAddress,
				},
				AccountDetails: jujuclient.AccountDetails{
					User:     username,
					Password: password,
				},
			},
		)
		if err != nil {
			return errors.E(fmt.Errorf("failed to run destroy-controller command: %w", err))
		}
		err = b.consumeCommandOutput(jobCtx, outputCh, jobId)
		if err != nil {
			return err
		}

		// Finally, remove controller if we have successfully destroyed it
		zapctx.Debug(jobCtx, "controller destroyed, removing from jimm")
		err = b.jujuManager.RemoveController(jobCtx, user, params.ControllerName, true)
		if err != nil {
			return errors.E(fmt.Errorf("failed to remove controller: %w", err))
		}

		return nil
	}
}
