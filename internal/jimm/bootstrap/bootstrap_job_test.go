// Copyright 2025 Canonical.
package bootstrap_test

import (
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
)

var (
	jobParams = bootstrap.JobParams{
		JujuDataDir:          "/path/to/a/juju/data/dir",
		CLIVersion:           "3.6.9",
		CLIOs:                "linux",
		CLIArch:              "aarch64",
		CloudNameAndRegion:   "special-cloud",
		ControllerName:       "a",
		AgentVersion:         "3.6.3",
		BootstrapTimeout:     0,
		CloudCred:            jujucloud.CloudCredential{},
		PersonalCloud:        jujucloud.Cloud{},
		LoginTokenRefreshURL: "jimm.com/.well-known/jwks.json",
	}
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

func (s *bootstrapManagerSuite) TestBootstrapJob(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	binary := &jujuclistore.Binary{FullPath: binaryPath}
	binary.RefCount().Store(1) // Simulate the Get().

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
	// TODO: Figure a way to check done is indeed deferred?
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	c.Assert(binary.RefCount().Load(), qt.Equals, int32(0))
}

func (s *bootstrapManagerSuite) TestBootstrapJob_FailsToLock(c *qt.C) {
	testCtx := c.Context()

	ctrl, store, jujuManager, binaryStore, executor, _, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	assertJobError(c, s, id, "failed to run bootstrap command: executor test error")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ReturnsEarlyIfLineErrors(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLineError := "command exited code 1"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
	// TODO: Figure a way to check done is indeed deferred?
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	assertJobError(c, s, id, "bootstrap command failed: command exited code 1")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetControllerDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
	// TODO: Figure a way to check done is indeed deferred?
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	assertJobError(c, s, id, "failed to get account details for controller a: client store failed to get account details")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetAccountDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
	// TODO: Figure a way to check done is indeed deferred?
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	assertJobError(c, s, id, "failed to get account details for controller a: account details test error")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_JujuManagerFailsToAddController(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, store, jujuManager, binaryStore, executor, clientStore, user := setupMocks(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(s.ofgaClient, store, s.jobTracker, jujuManager, binaryStore)
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
	// TODO: Figure a way to check done is indeed deferred?
	binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: jobParams.CLIVersion,
			Os:      jobParams.CLIVersion,
			Arch:    jobParams.CLIArch,
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
	assertJobError(c, s, id, "failed to add controller to JIMM: add controller test error")
	c.Assert(cleanupCalled, qt.IsTrue)
}
