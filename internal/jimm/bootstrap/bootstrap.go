// Copyright 2026 Canonical.

// bootstrap package provides functionality to manage the bootstrap process
// for controllers in JIMM.
package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/zaputil/zapctx"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/juju"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

var (
	binaryDone = func(b *jujuclistore.Binary) {
		b.Done()
	}
	// jujuCLILock ensures that only 1 routine (across bootstrap and destroy) uses the
	// Juju CLI at a time due to a global lock in Juju's store package used to access the
	// CLI data directory.
	// TODO: Create a more granular, safe-store implementation (see the TF provider).
	jujuCLILock = sync.Mutex{}
)

const (
	maxJobDuration = 60 * time.Minute
)

// Store defines the store methods required by the manager.
type Store interface {
	QueryJobLog(ctx context.Context, jobId int64, offset int) (loggies []string, nextOffsetValue int, err error)

	// BootstrapJob store methods:
	GetController(ctx context.Context, controller *dbmodel.Controller) (err error)
	AddJobLog(ctx context.Context, jobId int64, logLine string) (err error)
}

// JobQueue defines the method to enqueue a bootstrap/destroy-controller job.
type JobQueue interface {
	EnqueueBootstrap(ctx context.Context, args rivertypes.BootstrapArgs) (*rivertype.JobInsertResult, error)
	EnqueueDestroyController(ctx context.Context, args rivertypes.DestroyControllerArgs) (*rivertype.JobInsertResult, error)
	WaitForJobCompletion(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
	GetJobInfo(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
	CancelJob(ctx context.Context, jobID int64) (*rivertype.JobRow, error)
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
	jujuManager               JujuManager
	binaryStore               BinaryStore
	jimmWellknownJWKSEndpoint string
	credentialStore           CredentialStore
	jobQueue                  JobQueue
}

// NewBootstrapManager creates a new BootstrapManager instance.
func NewBootstrapManager(
	store Store,
	jobQueue JobQueue,
	jujuManager JujuManager,
	binaryStore BinaryStore,
	jimmWellknownJWKSEndpoint string,
	credentialStore CredentialStore,
) (*bootstrapManager, error) {
	if store == nil {
		return nil, errors.E("store cannot be nil")
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
	if jobQueue == nil {
		return nil, errors.E("job queue cannot be nil")
	}
	return &bootstrapManager{
		store:                     store,
		jujuManager:               jujuManager,
		binaryStore:               binaryStore,
		jimmWellknownJWKSEndpoint: jimmWellknownJWKSEndpoint,
		credentialStore:           credentialStore,
		jobQueue:                  jobQueue,
	}, nil
}

// GetJobInfo retrieves the status and logs of a bootstrap job.
// It requires the user to be an admin and returns the status, error message, logs,
// and a watermark for pagination.
func (b *bootstrapManager) GetJobInfo(ctx context.Context, _ *openfga.User, jobId int64, offset int) (params.GetJobInfoResponse, error) {
	job, err := b.jobQueue.GetJobInfo(ctx, jobId)
	if err != nil {
		return params.GetJobInfoResponse{}, fmt.Errorf("failed to get job info: %w", err)
	}
	logs, newOffset, err := b.store.QueryJobLog(ctx, jobId, offset)
	if err != nil {
		return params.GetJobInfoResponse{}, fmt.Errorf("failed to query job logs: %w", err)
	}
	var errorMsg strings.Builder
	for i, attemptErr := range job.Errors {
		fmt.Fprintf(&errorMsg, "attempt %d: %s\n", i, attemptErr.Error)
	}
	return params.GetJobInfoResponse{
		Status:    toParamsJobState(ctx, job.State),
		Error:     errorMsg.String(),
		Logs:      logs,
		Watermark: newOffset,
	}, nil
}

func toParamsJobState(ctx context.Context, state rivertype.JobState) params.JobStatus {
	switch state {
	case rivertype.JobStateCompleted:
		return params.StatusSuccessful

	case rivertype.JobStateRunning:
		return params.StatusRunning

	case rivertype.JobStateCancelled,
		rivertype.JobStateDiscarded:
		return params.StatusFailed

	case rivertype.JobStateAvailable,
		rivertype.JobStatePending,
		rivertype.JobStateScheduled,
		rivertype.JobStateRetryable:
		return params.StatusPending

	default:
		zapctx.Error(ctx, "unknown river job state", zap.String("state", string(state)))
		return params.StatusUnknown
	}
}

// StopJob stops a bootstrap job by its ID.
func (b *bootstrapManager) StopJob(ctx context.Context, user *openfga.User, jobID int64) error {

	if user == nil {
		return errors.E("user cannot be nil")
	}

	_, err := b.jobQueue.CancelJob(ctx, jobID)
	if err != nil {
		return errors.E("failed to stop job", err)
	}

	return nil
}

// WaitForJobCompletion waits for a bootstrap job to complete by polling its status.
// It returns an error if the job ID is nil, or if the job fails.
// It returns nil on successful completion.
func (b *bootstrapManager) WaitForJobCompletion(ctx context.Context, jobId int64, config WaitConfig) error {
	maxDuration := config.MaxJobDuration
	if maxDuration == 0 {
		maxDuration = maxJobDuration
	}

	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()

	job, err := b.jobQueue.WaitForJobCompletion(ctx, jobId)
	if err != nil {
		return fmt.Errorf("error while waiting for job completion: %w", err)
	}

	switch job.State {
	case rivertype.JobStateCompleted:
		return nil
	case rivertype.JobStateCancelled:
		return fmt.Errorf("bootstrap job was cancelled")
	case rivertype.JobStateDiscarded:
		if len(job.Errors) == 0 {
			return fmt.Errorf("bootstrap job failed")
		}
		lastErr := job.Errors[len(job.Errors)-1].Error
		return fmt.Errorf("bootstrap job failed: %s", lastErr)
	default:
		return fmt.Errorf("unexpected job state: %s", job.State)
	}
}

// StartBootstrap starts a bootstrap job with the provided parameters.
func (b *bootstrapManager) StartBootstrapJob(ctx context.Context, user *openfga.User, params BootstrapParams) (int64, error) {

	if b.jimmWellknownJWKSEndpoint == "" {
		return 0, errors.E("bootstrap login token refresh URL is not configured. Cannot proceed with bootstrap. Please configure it and try again.")
	}

	if err := params.validate(); err != nil {
		return 0, errors.E(fmt.Errorf("invalid bootstrap parameters: %v", err))
	}

	bootstrapArgs := rivertypes.BootstrapArgs{
		Username: user.Name,
		// Binary args.
		CLIVersion: params.CLIVersion,
		// User defined command arguments
		CloudNameAndRegion: params.CloudNameAndRegion,
		ControllerName:     params.ControllerName,
		CloudCred:          params.CloudCred,
		Cloud:              params.Cloud,
		// JIMM Provided command arguments (i.e., ones that must be set by JIMM when bootstrapping).
		LoginTokenRefreshURL: b.jimmWellknownJWKSEndpoint,
		// User defined config
		UserConfig: params.UserConfig,
	}
	job, err := b.jobQueue.EnqueueBootstrap(ctx, bootstrapArgs)
	if err != nil {
		return 0, errors.E(fmt.Errorf("failed to enqueue bootstrap job: %w", err))
	}
	if job.UniqueSkippedAsDuplicate {
		return 0, errors.E(fmt.Errorf("a bootstrap job is already in progress - please wait for it to complete before starting a new one"), errors.CodeInProgress)
	}

	return job.Job.ID, nil
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

// JujuCLI provides a concrete implementation of [CommandFactory]
// to be used in a [bootstrapManager.BootstrapJob] that uses
// a real Juju CLI.
type JujuCLI struct{}

// New create a new JujuCommands implementation.
func (h JujuCLI) New(binaryPath, jujuDataDir string) JujuCommands {
	return command{
		binaryPath:  binaryPath,
		jujuDataDir: jujuDataDir,
	}
}

// RunWrapper wraps the command runner and bootstrap command to be run, and then runs it for you.
// This enables the running portion of the BootstrapJob to be mocked.
func (h JujuCLI) RunWrapper(
	ctx context.Context,
	binaryPath, jujuDataDir string,
	params jujucommands.BootstrapCmdParams,
) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error) {
	r := jujucommands.NewCommandRunner(binaryPath, jujuDataDir)
	command := jujucommands.NewBootstrapCmd(r)
	return command.Run(ctx, params)
}

// BootstrapController bootstraps a new Juju controller and adds it to JIMM.
// It fetches a copy of the Juju CLI and uses that to execute bootstrap command.
func (b *bootstrapManager) BootstrapController(
	ctx context.Context,
	p RunBootstrapArgs,
	cmdFactory CommandFactory,
	user *openfga.User,
) error {
	// Lock the bootstrap concurrently with destroy to avoid misuse of the store commands.
	isLocked := jujuCLILock.TryLock()
	if !isLocked {
		return errors.E("another bootstrap or destroy operation is currently running, please wait for it to finish before starting a new one")
	}
	defer jujuCLILock.Unlock()

	// If we allow concurrent bootstraps, both could pass this check but only one would
	// succeed when trying to add the controller to JIMM.
	err := b.store.GetController(ctx, &dbmodel.Controller{Name: p.ControllerName})
	if err == nil {
		return errors.E(errors.CodeAlreadyExists, fmt.Errorf("controller %q already exists", p.ControllerName))
	}
	if errors.ErrorCode(err) != errors.CodeNotFound {
		return errors.E(fmt.Errorf("failed to check if controller exists: %w", err))
	}

	b.writeJobLog(ctx, p.JobID,
		fmt.Sprintf("Downloading the Juju CLI, version %s for bootstrap. This may take a few minutes", p.CLIVersion))

	binary, err := b.binaryStore.Get(
		ctx,
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		func(line string) {
			b.writeJobLog(ctx, p.JobID, line)
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
	}
	zapctx.Debug(ctx, "Juju binary downloaded, using Juju binary", zap.String("binary-path", binary.FullPath))
	defer binaryDone(binary)

	jujuCmds := cmdFactory.New(binary.FullPath, p.JujuDataDir)

	if err := b.runBootstrap(ctx, p, jujuCmds, user); err != nil {
		return errors.E(fmt.Errorf("run bootstrap failed: %w", err))
	}
	return nil
}

// runBootstrap wraps the logic of running a controller bootstrap for JIMM into
// a self-contained function. It is expected, and only expected to be run from within
// the [bootstrapManager.BootstrapJob].
//
// The ctx is expected to be the context of the job, and as such will be cancelled
// when the job is stopped or cancelled. The ctx is NOT expected to be used for
// any store operations, or other operations that should continue even if the job
// is cancelled.
func (b *bootstrapManager) runBootstrap(
	ctx context.Context,
	p RunBootstrapArgs,
	executor JujuCommands,
	user *openfga.User,
) error {

	outputCh, clientStore, cleanup, err := executor.Bootstrap(
		ctx,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
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
	// and log it while the command cancels the bootstrap while keeping things like
	// log info that was set on the context.
	ctx = context.WithoutCancel(ctx)

	err = b.consumeCommandOutput(ctx, outputCh, p.JobID)
	if err != nil {
		return err
	}

	// controllerCleanup is a helper function to cleanup the controller if we fail at any point
	// after the bootstrap, avoiding orphaned controllers in the cloud.
	controllerCleanup := func(err error, controllerDetails *jujuclient.ControllerDetails) error {
		cleanupErr := b.tryCleanupController(ctx, executor, p.JobID, p.ControllerName)
		if cleanupErr == nil {
			return errors.E(fmt.Errorf("error post-bootstrap: %w\n"+
				"the controller has been automatically destroyed", err))
		}
		var controllerDetailsStr string
		if controllerDetails != nil {
			res, _ := yaml.Marshal(controllerDetails)
			controllerDetailsStr = string(res)
		}

		zapctx.Error(ctx, "failed to cleanup controller after failing to add it to JIMM",
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
		ctx,
		user,
		&dbCtrl,
		dbCtrlCreds,
	); err != nil {
		return controllerCleanup(fmt.Errorf("failed to add controller to JIMM: %w", err), ctrlDetails)
	}

	return nil
}

func (b *bootstrapManager) tryCleanupController(ctx context.Context, jujuCmd JujuCommands, jobID int64, controllerName string) error {
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

func (b *bootstrapManager) consumeCommandOutput(ctx context.Context, outputCh <-chan jujucommands.OutputLine, jobId int64) error {
	for output := range outputCh {
		if output.Err != nil {
			b.writeJobLog(ctx, jobId, output.Err.Error())
			return errors.E(fmt.Errorf("command failed: %w", output.Err))
		}
		zapctx.Debug(ctx, "command output", zap.Int64("job-id", jobId), zap.String("line", output.Line))
		b.writeJobLog(ctx, jobId, output.Line)
	}
	return nil
}

// writeJobLog writes logs to the store to eventually be displayed to users.
// Errors are masked but logged to avoid failing the bootstrap process.
func (b *bootstrapManager) writeJobLog(ctx context.Context, jobId int64, logLine string) {
	// Avoid storing empty log lines.
	if logLine == "" {
		return
	}
	if err := b.store.AddJobLog(ctx, jobId, logLine); err != nil {
		zapctx.Error(ctx, "failed to write bootstrap log", zap.Error(err), zap.Int64("jobId", jobId))
	}
}

// StartDestroyControllerJob inserts a destroy-controller job into the database.
func (b *bootstrapManager) StartDestroyControllerJob(ctx context.Context, user *openfga.User, params DestroyControllerParams) (int64, error) {
	destroyArgs := rivertypes.DestroyControllerArgs{
		Username:       user.Name,
		ControllerName: params.ControllerName,
		ControllerUUID: params.ControllerUUID,
		AgentVersion:   params.AgentVersion,
		CloudName:      params.CloudName,
		CloudRegion:    params.CloudRegion,
		APIEndpoints:   params.APIEndpoints,
		CACertificate:  params.CACertificate,
		PublicAddress:  params.PublicAddress,
	}

	job, err := b.jobQueue.EnqueueDestroyController(ctx, destroyArgs)
	if err != nil {
		return 0, errors.E(fmt.Errorf("failed to start bootstrap job: %w", err))
	}
	if job.UniqueSkippedAsDuplicate {
		return 0, errors.E(fmt.Errorf("a destroy job is already in progress - please wait for it to complete before starting a new one"), errors.CodeInProgress)
	}

	return job.Job.ID, nil
}

// DestroyController destroys a Juju controller and removes it from JIMM.
// It fetches a copy of the Juju CLI and uses that to execute destroy-controller command.
func (b *bootstrapManager) DestroyController(
	ctx context.Context,
	p RunDestroyControllerArgs,
	cmdFactory CommandFactory,
	user *openfga.User,
) error {
	// Lock the destroy concurrently with bootstrap to avoid misuse of the store commands.
	isLocked := jujuCLILock.TryLock()
	if !isLocked {
		return errors.E("another bootstrap or destroy operation is currently running, please wait for it to finish before starting a new one")
	}
	defer jujuCLILock.Unlock()

	b.writeJobLog(ctx, p.JobID,
		fmt.Sprintf("Downloading the Juju CLI, version %s for destroy-controller. This may take a few minutes", p.AgentVersion))

	binary, err := b.binaryStore.Get(
		ctx,
		jujuclistore.JujuBinarySpec{
			Version: p.AgentVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		func(line string) {
			b.writeJobLog(ctx, p.JobID, line)
		},
	)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get Juju binary: %w", err))
	}
	zapctx.Debug(ctx, "Juju binary downloaded, using Juju binary", zap.String("binary-path", binary.FullPath))
	defer func() {
		binary.Done()
	}()

	jujuCmd := cmdFactory.New(binary.FullPath, p.JujuDataDir)

	username, password, err := b.credentialStore.GetControllerCredentials(ctx, p.ControllerName)
	if err != nil {
		return errors.E(fmt.Errorf("failed to get controller credentials: %w", err))
	}

	// Update the context from this point to prevent it from being cancelled when the parent is cancelled.
	// This ensures that we still capture output from the destroy-controller command
	// and log it if that command is cancelled while keeping things like
	// log info that was set on the context.
	ctx = context.WithoutCancel(ctx)

	outputCh, err := jujuCmd.DestroyController(
		ctx,
		jujucommands.DestroyControllerCmdParams{
			ControllerName: p.ControllerName,
			ControllerDetails: jujuclient.ControllerDetails{
				ControllerUUID: p.ControllerUUID,
				Cloud:          p.CloudName,
				CloudRegion:    p.CloudRegion,
				APIEndpoints:   p.APIEndpoints,
				CACert:         p.CACertificate,
				PublicDNSName:  p.PublicAddress,
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
	err = b.consumeCommandOutput(ctx, outputCh, p.JobID)
	if err != nil {
		return err
	}

	// Finally, remove controller if we have successfully destroyed it
	zapctx.Debug(ctx, "controller destroyed, removing from jimm")
	err = b.jujuManager.RemoveController(ctx, user, p.ControllerName, true)
	if err != nil {
		return errors.E(fmt.Errorf("failed to remove controller: %w", err))
	}

	return nil
}
