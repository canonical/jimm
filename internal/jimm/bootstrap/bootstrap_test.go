// Copyright 2026 Canonical.

package bootstrap_test

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	"github.com/google/uuid"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	bootstrapmocks "github.com/canonical/jimm/v3/internal/jimm/bootstrap/mocks"
	"github.com/canonical/jimm/v3/internal/jobtracker"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/testdb"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapManagerSuite struct {
	jobTracker *jobtracker.Tracker
	adminUser  *openfga.User
	db         *db.Database
}

var (
	jobParams = bootstrap.JobParams{
		JujuDataDir: "/path/to/a/juju/data/dir",

		CLIVersion: "3.6.9",

		CloudNameAndRegion: "special-cloud",
		ControllerName:     "a",
		AgentVersion:       "3.6.3",

		CloudCred: jujucloud.Credential{},
		Cloud:     jujucloud.Cloud{},

		LoginTokenRefreshURL: loginTokenRefreshURLParam,
	}
	//nolint:gosec
	loginTokenRefreshURLParam = "jimm.com/.well-known/jwks.json"
)

func pollJob(c *qt.C, s *bootstrapManagerSuite, id uuid.UUID, expectedStatus dbmodel.JobStatus) {
	var status dbmodel.JobStatus
	var pollerr error
	for i := 0; i < 20; i++ {
		status, pollerr = s.db.GetJobStatus(c.Context(), id)
		c.Assert(pollerr, qt.IsNil)
		if status == expectedStatus {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.Assert(status, qt.Equals, expectedStatus)
}

func assertJobError(c *qt.C, s *bootstrapManagerSuite, id uuid.UUID, errStr string) {
	entry := &dbmodel.JobTrackerEntry{JobID: id}
	err := s.db.GetJob(c.Context(), entry)
	c.Assert(err, qt.IsNil)
	c.Assert(entry.Error, qt.Equals, errStr)
}

type bootstrapMocks struct {
	store           *bootstrapmocks.MockStore
	jujuManager     *bootstrapmocks.MockJujuManager
	binaryStore     *bootstrapmocks.MockBinaryStore
	commandFactory  *bootstrapmocks.MockCommandFactory
	clientStore     *bootstrapmocks.MockClientStore
	executor        *bootstrapmocks.MockJujuCommands
	credentialStore *bootstrapmocks.MockCredentialStore
}

func setupTest(c *qt.C) (
	*gomock.Controller,
	bootstrapMocks,
	*openfga.User,
) {
	ctrl := gomock.NewController(c)

	m := bootstrapMocks{
		store:           bootstrapmocks.NewMockStore(ctrl),
		jujuManager:     bootstrapmocks.NewMockJujuManager(ctrl),
		binaryStore:     bootstrapmocks.NewMockBinaryStore(ctrl),
		commandFactory:  bootstrapmocks.NewMockCommandFactory(ctrl),
		clientStore:     bootstrapmocks.NewMockClientStore(ctrl),
		executor:        bootstrapmocks.NewMockJujuCommands(ctrl),
		credentialStore: bootstrapmocks.NewMockCredentialStore(ctrl),
	}

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(i, nil)

	return ctrl, m, user
}

func (s *bootstrapManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: testdb.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	jobtracker, err := jobtracker.New(db, 1*time.Minute)
	s.jobTracker = jobtracker
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLNotConfigured(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()
	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, "", mocks.credentialStore)
	c.Assert(err, qt.IsNil)
	_, err = manager.StartBootstrapJob(ctx, user, bootstrap.BootstrapParams{})
	c.Assert(err, qt.ErrorMatches, "bootstrap login token refresh URL is not configured.*")
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLEmpty(c *qt.C) {
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()
	_, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, "", mocks.credentialStore)
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLMalformed(c *qt.C) {
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()
	_, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, ":/url/", mocks.credentialStore)
	c.Assert(err, qt.ErrorMatches, ".*failed to parse bootstrap login token refresh URL.*")
}

func (s *bootstrapManagerSuite) TestGetJobInfo(c *qt.C) {
	ctx := c.Context()
	read := make(chan struct{})
	defer close(read)
	write := make(chan struct{})
	defer close(write)

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	numLogs := 101
	batchSize := 10

	jobId, err := s.jobTracker.Run(ctx,
		"bootstrap-job",
		func(ctx context.Context) error {
			jobId, ok := jobtracker.JobIdFromContext(ctx)
			c.Check(ok, qt.IsTrue)

			for i := range numLogs {
				if i%batchSize == 0 && i > 0 {
					write <- struct{}{} // Signal that a batch of logs has been written.
					<-read              // Wait for the read before writing the next batch.
				}
				err := s.db.AddJobLog(ctx, jobId, "bootstrap logs "+fmt.Sprint(rune(i)))
				c.Check(err, qt.IsNil)
			}
			// We need to signal that we've written the last batch of logs.
			write <- struct{}{}
			<-read
			return nil
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)
	watermark := 0
	for batch := 0; batch < numLogs/batchSize+1; batch++ {
		<-write // Wait for the batch of logs to be written.

		response, err := manager.GetJobInfo(ctx, s.adminUser, jobId, watermark)
		c.Assert(err, qt.IsNil)
		logs := []string{}
		for j := 0; j < int(math.Min(float64(batchSize), float64(numLogs-batch*batchSize))); j++ {
			logs = append(logs, "bootstrap logs "+fmt.Sprint(rune(batch*batchSize+j)))
		}
		c.Check(response.Logs, qt.DeepEquals, logs)
		c.Assert(response.Status, qt.Equals, params.StatusRunning)
		watermark = response.Watermark
		read <- struct{}{} // Signal it has been read.
	}

	// check last batch is empty.
	response, err := manager.GetJobInfo(ctx, s.adminUser, jobId, watermark)
	c.Assert(response.Status == params.StatusSuccessful || response.Status == params.StatusRunning, qt.IsTrue)
	c.Assert(err, qt.IsNil)
	c.Assert(response.Logs, qt.HasLen, 0)
}

func (s *bootstrapManagerSuite) TestGetJobInfo_JobFailed(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobId, err := s.jobTracker.Run(ctx,
		"bootstrap-job",
		func(ctx context.Context) error {
			return fmt.Errorf("I died really fast")
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)
	var response params.GetJobInfoResponse
	for range 10 {
		response, err = manager.GetJobInfo(ctx, s.adminUser, jobId, 0)
		c.Assert(err, qt.IsNil)
		if response.Status == params.StatusFailed {
			break
		}
		time.Sleep(100 * time.Millisecond) // Wait for the job to be marked as failed.
	}
	c.Assert(response.Status, qt.Equals, params.StatusFailed)
	c.Assert(response.Error, qt.Equals, "I died really fast")
}

func (s *bootstrapManagerSuite) TestGetJobInfo_JobNotFound(c *qt.C) {
	ctx := c.Context()
	jobId := uuid.New()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	_, err = manager.GetJobInfo(ctx, s.adminUser, jobId, 0)
	c.Assert(err, qt.ErrorMatches, "failed to get job info")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_NilJobID(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	err = manager.WaitForJobCompletion(ctx, uuid.Nil, bootstrap.WaitConfig{})
	c.Assert(err, qt.ErrorMatches, ".*job ID cannot be nil")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_Success(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobId, err := s.jobTracker.Run(ctx,
		"test-job",
		func(ctx context.Context) error {
			return nil
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)

	err = manager.WaitForJobCompletion(ctx, jobId, bootstrap.WaitConfig{})
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_JobFails(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobId, err := s.jobTracker.Run(ctx,
		"test-job",
		func(ctx context.Context) error {
			return errors.E("job execution failed")
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)

	err = manager.WaitForJobCompletion(ctx, jobId, bootstrap.WaitConfig{})
	c.Assert(err, qt.ErrorMatches, ".*bootstrap job failed: job execution failed")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_JobNotFound(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobId := uuid.New()

	err = manager.WaitForJobCompletion(ctx, jobId, bootstrap.WaitConfig{})
	c.Assert(err, qt.ErrorMatches, ".*failed to get job info.*")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_Timeout(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Create a job that will keep running (never completes)
	jobId, err := s.jobTracker.Run(ctx,
		"test-job",
		func(ctx context.Context) error {
			// Block until context is cancelled
			<-ctx.Done()
			return ctx.Err()
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)

	// Wait with a very short timeout and fast polling
	err = manager.WaitForJobCompletion(ctx, jobId, bootstrap.WaitConfig{
		MaxJobDuration:  100 * time.Millisecond,
		PollingInterval: 10 * time.Millisecond,
	})
	c.Assert(err, qt.ErrorMatches, ".*job completion wait timed out")
}

func (s *bootstrapManagerSuite) TestBootstrapJob(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	binary := &jujuclistore.Binary{FullPath: binaryPath}
	binaryDoneCalled := false

	c.Patch(bootstrap.BinaryDone, func(b *jujuclistore.Binary) {
		binaryDoneCalled = true
		binary.Done()
	})

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		binary,
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	ctrlDetails := &jujuclient.ControllerDetails{
		APIEndpoints: []string{
			"10.0.0.1:17070",
			"172.0.0.1:17070",
			"192.0.0.1:17070",
		},
		ControllerUUID: "I am actually a uuid, I promise",
		PublicDNSName:  "I am not a public DNS, I am a private DNS",
		CACert:         "Very secure CA cert, promise",
	}
	mocks.clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	mocks.jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          jobParams.ControllerName,
			PublicAddress: ctrlDetails.PublicDNSName,
			CACertificate: ctrlDetails.CACert,
			Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
			TLSHostname:   "juju-apiserver",
		},
		gomock.Any(),
	).Return(nil)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusSuccessful)
	c.Assert(cleanupCalled, qt.IsTrue)
	// Check binary is no longer referenced.
	c.Assert(binaryDoneCalled, qt.Equals, true)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_FailsToLock(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(errors.E("bootstrap lock is already held"))

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "failed to acquire bootstrap lock: bootstrap lock is already held")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ControllerExists(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(nil)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, `controller "a" already exists`)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ControllerRetrievalFails(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(errors.E("oh noes, we couldnt'se get the controller"))
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "failed to check if controller exists: oh noes, we couldnt'se get the controller")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_BinaryStoreGetFails(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		nil,
		errors.E("test error"),
	)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "failed to get Juju binary: test error")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ExecutorFails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			return nil
		}(),
		mocks.clientStore,
		func() {},
		errors.E("executor test error"),
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "run bootstrap failed: failed to run bootstrap command: executor test error")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ReturnsEarlyIfLineErrors(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLineError := "command exited code 1"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Err: errors.E(testOutputLineError)}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "run bootstrap failed: command failed: command exited code 1")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetControllerDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	ctrlDetails := &jujuclient.ControllerDetails{
		APIEndpoints: []string{
			"10.0.0.1:17070",
			"172.0.0.1:17070",
			"192.0.0.1:17070",
		},
		ControllerUUID: "I am actually a uuid, I promise",
		PublicDNSName:  "I am not a public DNS, I am a private DNS",
		CACert:         "Very secure CA cert, promise",
	}
	mocks.clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		nil,
		errors.E("client store failed to get account details"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: jobParams.ControllerName,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		nil,
	)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c,
		s,
		id,
		"run bootstrap failed: error post-bootstrap: failed to get account details for controller a: client store failed to get account details\n"+
			"the controller has been automatically destroyed")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetAccountDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	ctrlDetails := &jujuclient.ControllerDetails{
		APIEndpoints: []string{
			"10.0.0.1:17070",
			"172.0.0.1:17070",
			"192.0.0.1:17070",
		},
		ControllerUUID: "I am actually a uuid, I promise",
		PublicDNSName:  "I am not a public DNS, I am a private DNS",
		CACert:         "Very secure CA cert, promise",
	}
	mocks.clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		nil,
		errors.E("account details test error"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: jobParams.ControllerName,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		nil,
	)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c,
		s,
		id,
		"run bootstrap failed: error post-bootstrap: failed to get account details for controller a: account details test error\n"+
			"the controller has been automatically destroyed")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_JujuManagerFailsToAddController(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	ctrlDetails := &jujuclient.ControllerDetails{
		APIEndpoints: []string{
			"10.0.0.1:17070",
			"172.0.0.1:17070",
			"192.0.0.1:17070",
		},
		ControllerUUID: "I am actually a uuid, I promise",
		PublicDNSName:  "I am not a public DNS, I am a private DNS",
		CACert:         "Very secure CA cert, promise",
	}
	mocks.clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	mocks.jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          jobParams.ControllerName,
			PublicAddress: ctrlDetails.PublicDNSName,
			CACertificate: ctrlDetails.CACert,
			Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
			TLSHostname:   "juju-apiserver",
		},
		gomock.Any(),
	).Return(
		errors.E("add controller test error"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: jobParams.ControllerName,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		nil,
	)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c,
		s,
		id,
		"run bootstrap failed: error post-bootstrap: failed to add controller to JIMM: add controller test error\n"+
			"the controller has been automatically destroyed")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_CleanupControllerFailure(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		mocks.clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	)
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	ctrlDetails := &jujuclient.ControllerDetails{
		APIEndpoints: []string{
			"10.0.0.1:17070",
			"172.0.0.1:17070",
			"192.0.0.1:17070",
		},
		ControllerUUID: "I am actually a uuid, I promise",
		PublicDNSName:  "I am not a public DNS, I am a private DNS",
		CACert:         "Very secure CA cert, promise",
	}
	mocks.clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	mocks.jujuManager.EXPECT().AddController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(
		errors.E("add controller test error"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: jobParams.ControllerName,
		},
	).Return(
		nil,
		errors.E("cleanup controller test failure"),
	)
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c,
		s,
		id,
		"run bootstrap failed: error post-bootstrap: failed to add controller to JIMM: add controller test error\n"+
			"automatic cleanup of the controller also failed: failed to run destroy-controller command: cleanup controller test failure\n"+
			"\n"+
			"WARNING: resources associated with the controller may remain dangling in your environment.\n"+
			"Manual intervention is required, either attach the controller to JIMM or destroy it.\n"+
			"\n"+
			"Controller details:\n"+
			"uuid: I am actually a uuid, I promise\n"+
			"api-endpoints: ['10.0.0.1:17070', '172.0.0.1:17070', '192.0.0.1:17070']\n"+
			"public-hostname: I am not a public DNS, I am a private DNS\n"+
			"ca-cert: Very secure CA cert, promise\n"+
			"cloud: \"\"\n"+
			"controller-machine-count: 0\n"+
			"active-controller-machine-count: 0\n")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_CancelledJob(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	cancelledBootstrapLine := "bootstrap-cancelled"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	// create a new job tracker with a lower polling interval.
	tracker, err := jobtracker.New(s.db, 1*time.Second)
	c.Assert(err, qt.IsNil)
	s.jobTracker = tracker

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	)
	mocks.executor.EXPECT().Bootstrap(
		gomock.Any(),
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			DefaultLoginTokenURL: jobParams.LoginTokenRefreshURL,
			Cloud:                jobParams.Cloud,
			CloudCred:            jobParams.CloudCred,
		},
	).DoAndReturn(func(ctx context.Context, bcp jujucommands.BootstrapCmdParams) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error) {
		// Cancel the job to simulate a user cancelling bootstrap.
		jobId, ok := jobtracker.JobIdFromContext(ctx)
		c.Check(ok, qt.IsTrue)
		err := s.jobTracker.StopJob(ctx, jobId)
		c.Check(err, qt.IsNil)

		// Wait to ensure the context we've received gets cancelled.
		// If the context passed to Bootstrap is not the job context, this will timeout and fail the test.
		// The job tracker polls the DB to detect whether the job has stopped so we wait a bit longer than the poll interval.
		select {
		case <-ctx.Done():
		case <-time.After(time.Second * 5):
			c.Error("expected context to be cancelled")
		}

		output := func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: cancelledBootstrapLine, Err: errors.E("failed-bootstrap")}
			close(outputCh)
			return outputCh
		}()
		cleanup := func() {
			cleanupCalled = true
		}
		return output, mocks.clientStore, cleanup, nil
	})
	mocks.commandFactory.EXPECT().New(binaryPath, jobParams.JujuDataDir).
		Return(mocks.executor)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, u uuid.UUID, s string) error {
			if err := ctx.Err(); err != nil {
				c.Errorf("expected valid context, got error: %v", err)
				c.Log("This may indicate we are using an incorrect context.")
			}
			return nil
		}).AnyTimes()

	bootstrapDone := make(chan struct{})
	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).
		DoAndReturn(func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				c.Errorf("expected valid context, got error: %v", err)
				c.Log("This may indicate we are using an incorrect context.")
			}
			close(bootstrapDone)
			return nil
		})

	job := manager.BootstrapJob(
		jobParams,
		mocks.commandFactory,
		user,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	select {
	case <-bootstrapDone:
	case <-time.After(time.Second * 10):
		c.Fatal("timed out waiting for bootstrap to complete")
	}
	pollJob(c, s, id, dbmodel.StatusFailed)
	assertJobError(c, s, id, "run bootstrap failed: command failed: failed-bootstrap")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestDestroyControllerJob(c *qt.C) {
	testCtx := c.Context()

	cliVersion := "3.6.9"
	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"
	controllerName := "moribund"

	binary := &jujuclistore.Binary{FullPath: binaryPath}
	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, s.jobTracker, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	mocks.store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: cliVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		binary,
		nil,
	)

	mocks.commandFactory.EXPECT().New(binaryPath, gomock.Any()).Return(mocks.executor)
	mocks.credentialStore.EXPECT().GetControllerCredentials(gomock.Any(), controllerName).Return("username", "password", nil)

	mocks.executor.EXPECT().DestroyController(gomock.Any(), jujucommands.DestroyControllerCmdParams{
		ControllerName:    controllerName,
		ControllerDetails: jujuclient.ControllerDetails{},
		AccountDetails: jujuclient.AccountDetails{
			User:     "username",
			Password: "password",
		},
	}).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		nil,
	)

	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mocks.jujuManager.EXPECT().RemoveController(gomock.Any(), gomock.Any(), controllerName, true)

	mocks.store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil)

	job := manager.DestroyControllerJob(
		bootstrap.DestroyControllerParams{
			ControllerName: controllerName,
			AgentVersion:   cliVersion,
		},
		mocks.commandFactory,
		user,
		binaryPath,
	)

	id, err := s.jobTracker.Run(
		testCtx,
		"test-job-type",
		job,
		time.Second*1000,
	)
	c.Assert(err, qt.IsNil)

	pollJob(c, s, id, dbmodel.StatusSuccessful)
}

//go:generate go tool mockgen -typed -destination=./mocks/store.go -package=mocks . Store
//go:generate go tool mockgen -typed -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate go tool mockgen -typed -destination=./mocks/binarystore.go -package=mocks . BinaryStore
//go:generate go tool mockgen -typed -destination=./mocks/commandfactory.go -package=mocks . CommandFactory
//go:generate go tool mockgen -typed -destination=./mocks/jujucommands.go -package=mocks . JujuCommands
//go:generate go tool mockgen -typed -destination=./mocks/jujuclientstore.go -package=mocks github.com/juju/juju/jujuclient ClientStore
//go:generate go tool mockgen -typed -destination=./mocks/credentialstore.go -package=mocks . CredentialStore
func TestBootstrapManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &bootstrapManagerSuite{})
}
