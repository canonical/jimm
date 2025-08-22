// Copyright 2025 Canonical.

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
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap/mocks"
	"github.com/canonical/jimm/v3/internal/jobtracker"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
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
		BootstrapTimeout:   0,

		CloudCred:     jujucloud.CloudCredential{},
		PersonalCloud: jujucloud.Cloud{},

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

func setupMocks(c *qt.C) (
	*gomock.Controller,
	*mocks.MockStore,
	*mocks.MockJujuManager,
	*mocks.MockBinaryStore,
	*mocks.MockBootstrapExecutor,
	*mocks.MockClientStore,
	*openfga.User,
) {
	ctrl := gomock.NewController(c)

	store := mocks.NewMockStore(ctrl)
	jujuManager := mocks.NewMockJujuManager(ctrl)
	binaryStore := mocks.NewMockBinaryStore(ctrl)
	executor := mocks.NewMockBootstrapExecutor(ctrl)
	clientStore := mocks.NewMockClientStore(ctrl)

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(i, nil)

	return ctrl, store, jujuManager, binaryStore, executor, clientStore, user
}

func (s *bootstrapManagerSuite) Init(c *qt.C) {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := db.Migrate(context.Background())
	c.Assert(err, qt.IsNil)

	s.db = db

	jobtracker, err := jobtracker.New(db, 1*time.Minute)
	s.jobTracker = jobtracker
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs(c *qt.C) {
	ctx := c.Context()
	read := make(chan struct{})
	defer close(read)
	write := make(chan struct{})
	defer close(write)

	ctrl, _, jujuManager, binaryStore, _, _, _ := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
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
				err := s.db.AddBootstrapLog(ctx, jobId, "bootstrap logs "+fmt.Sprint(rune(i)))
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

		response, err := manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, watermark)
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
	response, err := manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, watermark)
	c.Assert(response.Status == params.StatusSuccessful || response.Status == params.StatusRunning, qt.IsTrue)
	c.Assert(err, qt.IsNil)
	c.Assert(response.Logs, qt.HasLen, 0)
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs_JobFailed(c *qt.C) {
	ctx := c.Context()
	ctrl, _, jujuManager, binaryStore, _, _, _ := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	jobId, err := s.jobTracker.Run(ctx,
		"bootstrap-job",
		func(ctx context.Context) error {
			return fmt.Errorf("I died really fast")
		},
		1*time.Minute,
	)
	c.Assert(err, qt.IsNil)
	var response params.BootstrapStatusResponse
	for range 10 {
		response, err = manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, 0)
		c.Assert(err, qt.IsNil)
		if response.Status == params.StatusFailed {
			break
		}
		time.Sleep(100 * time.Millisecond) // Wait for the job to be marked as failed.
	}
	c.Assert(response.Status, qt.Equals, params.StatusFailed)
	c.Assert(response.Error, qt.Equals, "I died really fast")
}

func (s *bootstrapManagerSuite) TestGetBootstrapStatusAndLogs_JobNotFound(c *qt.C) {
	ctx := c.Context()
	jobId := uuid.New()

	ctrl, _, jujuManager, binaryStore, _, _, _ := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.db, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	_, err = manager.GetBootstrapStatusAndLogs(ctx, s.adminUser, jobId, 0)
	c.Assert(err, qt.ErrorMatches, "failed to get job status")
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

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		binary,
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	).Times(1)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	store.EXPECT().AddBootstrapLog(gomock.Any(), gomock.Any(), testOutputLine).Return(nil).Times(1)
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
	clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	).Times(1)
	clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          jobParams.ControllerName,
			PublicAddress: ctrlDetails.PublicDNSName,
			CACertificate: ctrlDetails.CACert,
			Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
		},
		gomock.Any(),
	).Return(nil).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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

	ctrl, store, jujuManager, binaryStore, executor, _, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(errors.E("bootstrap lock is already held")).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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

	ctrl, store, jujuManager, binaryStore, executor, _, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(nil).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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

	ctrl, store, jujuManager, binaryStore, executor, _, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(errors.E("oh noes, we couldnt'se get the controller")).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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

	ctrl, store, jujuManager, binaryStore, executor, _, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		nil,
		errors.E("test error"),
	).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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

func (s *bootstrapManagerSuite) TestBootstrapJob_ExecutorRunWrapperFails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			return nil
		}(),
		clientStore,
		func() {},
		errors.E("executor test error"),
	).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,

		executor,
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

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Err: errors.E(testOutputLineError)}
			close(outputCh)
			return outputCh
		}(),
		clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	).Times(1)
	store.EXPECT().AddBootstrapLog(gomock.Any(), gomock.Any(), testOutputLineError).Return(nil).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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
	assertJobError(c, s, id, "run bootstrap failed: bootstrap command failed: command exited code 1")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetControllerDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	).Times(1)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	store.EXPECT().AddBootstrapLog(gomock.Any(), gomock.Any(), testOutputLine).Return(nil).Times(1)
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
	clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	).Times(1)
	clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		nil,
		errors.E("client store failed to get account details"),
	)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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
	assertJobError(c, s, id, "run bootstrap failed: failed to get account details for controller a: client store failed to get account details")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetAccountDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	).Times(1)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	store.EXPECT().AddBootstrapLog(gomock.Any(), gomock.Any(), testOutputLine).Return(nil).Times(1)
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
	clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	).Times(1)
	clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		nil,
		errors.E("account details test error"),
	)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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
	assertJobError(c, s, id, "run bootstrap failed: failed to get account details for controller a: account details test error")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_JujuManagerFailsToAddController(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(store, s.jobTracker, jujuManager, binaryStore, loginTokenRefreshURLParam)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	store.EXPECT().LockBootstrap(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: jobParams.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	).Times(1)
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
	).Return(
		&jujuclistore.Binary{FullPath: binaryPath},
		nil,
	).Times(1)
	executor.EXPECT().RunWrapper(
		gomock.Any(),
		binaryPath,
		jobParams.JujuDataDir,
		jujucommands.BootstrapCmdParams{
			CloudNameAndRegion:   jobParams.CloudNameAndRegion,
			ControllerName:       jobParams.ControllerName,
			AgentVersion:         jobParams.AgentVersion,
			BootstrapTimeout:     jobParams.BootstrapTimeout,
			LoginTokenRefreshURL: jobParams.LoginTokenRefreshURL,
			PersonalCloud:        jobParams.PersonalCloud,
			CloudCred:            jobParams.CloudCred,
		},
	).Return(
		func() chan jujucommands.OutputLine {
			outputCh := make(chan jujucommands.OutputLine, 1)
			outputCh <- jujucommands.OutputLine{Line: testOutputLine}
			close(outputCh)
			return outputCh
		}(),
		clientStore,
		func() {
			cleanupCalled = true
		},
		nil,
	).Times(1)
	// We don't know the jobid to expect it yet. I did test by moving this line below the call, and it does
	// pass, but it'd be racey between the starting of the job routine and the EXPECT.
	store.EXPECT().AddBootstrapLog(gomock.Any(), gomock.Any(), testOutputLine).Return(nil).Times(1)
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
	clientStore.EXPECT().ControllerByName(jobParams.ControllerName).Return(
		ctrlDetails,
		nil,
	).Times(1)
	clientStore.EXPECT().AccountDetails(jobParams.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          jobParams.ControllerName,
			PublicAddress: ctrlDetails.PublicDNSName,
			CACertificate: ctrlDetails.CACert,
			Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
		},
		gomock.Any(),
	).Return(
		errors.E("add controller test error"),
	).Times(1)
	store.EXPECT().UnlockBootstrap(gomock.Any()).Return(nil).Times(1)

	job := manager.BootstrapJob(
		jobParams,
		executor,
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
	assertJobError(c, s, id, "run bootstrap failed: failed to add controller to JIMM: add controller test error")
	c.Assert(cleanupCalled, qt.IsTrue)
}

//go:generate mockgen -destination=./mocks/store.go -package=mocks . Store
//go:generate mockgen -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate mockgen -destination=./mocks/binarystore.go -package=mocks . BinaryStore
//go:generate mockgen -destination=./mocks/bootstrapexecutor.go -package=mocks . BootstrapExecutor
//go:generate mockgen -destination=./mocks/jujuclientstore.go -package=mocks github.com/juju/juju/jujuclient ClientStore
func TestBootstrapManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &bootstrapManagerSuite{})
}
