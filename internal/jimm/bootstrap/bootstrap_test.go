// Copyright 2026 Canonical.

package bootstrap_test

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"strings"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/frankban/quicktest/qtsuite"
	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/errors"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	bootstrapmocks "github.com/canonical/jimm/v3/internal/jimm/bootstrap/mocks"
	"github.com/canonical/jimm/v3/internal/jujuclistore"
	"github.com/canonical/jimm/v3/internal/jujucommands"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
	"github.com/canonical/jimm/v3/pkg/api/params"
)

type bootstrapManagerSuite struct {
	adminUser *openfga.User
}

const (
	defaultJobID int64 = 123
	//nolint:gosec
	loginTokenRefreshURLParam = "https://jimm.com/.well-known/jwks.json"
)

func defaultBootstrapArgs() bootstrap.RunBootstrapArgs {
	return bootstrap.RunBootstrapArgs{
		RunnerArgs: bootstrap.RunnerArgs{
			JujuDataDir: "/path/to/a/juju/data/dir",
			JobID:       defaultJobID,
		},
		BootstrapArgs: rivertypes.BootstrapArgs{
			Username:             "bob@canonical.com",
			CLIVersion:           "3.6.9",
			CloudNameAndRegion:   "special-cloud",
			ControllerName:       "a",
			AgentVersion:         "3.6.3",
			CloudCred:            jujucloud.Credential{},
			Cloud:                jujucloud.Cloud{},
			LoginTokenRefreshURL: loginTokenRefreshURLParam,
			BootstrapOptions:     defaultRiverBootstrapOptions(),
		},
	}
}

func defaultBootstrapOptions() bootstrap.BootstrapOptions {
	return bootstrap.BootstrapOptions{
		BootstrapBase:        "ubuntu@24.04",
		BootstrapConstraints: map[string]string{"mem": "8G"},
		ModelConstraints:     map[string]string{"arch": "amd64"},
		ModelDefault:         map[string]string{"logging-config": "<root>=INFO"},
		StoragePool: &bootstrap.StoragePool{
			Name:       "controller-pool",
			Type:       "ebs",
			Attributes: map[string]string{"volume-type": "gp3"},
		},
		BootstrapConfig:       map[string]string{"foo": "bar"},
		ControllerConfig:      map[string]string{"audit-log-enabled": "true"},
		ControllerModelConfig: map[string]string{"image-stream": "released"},
	}
}

func defaultRiverBootstrapOptions() rivertypes.BootstrapOptions {
	return expectedRiverBootstrapOptions(defaultBootstrapOptions())
}

func expectedRiverBootstrapOptions(options bootstrap.BootstrapOptions) rivertypes.BootstrapOptions {
	return rivertypes.BootstrapOptions{
		BootstrapBase:        options.BootstrapBase,
		BootstrapConstraints: options.BootstrapConstraints,
		ModelConstraints:     options.ModelConstraints,
		ModelDefault:         options.ModelDefault,
		StoragePool: &rivertypes.BootstrapStoragePool{
			Name:       options.StoragePool.Name,
			Type:       options.StoragePool.Type,
			Attributes: options.StoragePool.Attributes,
		},
		BootstrapConfig:       options.BootstrapConfig,
		ControllerConfig:      options.ControllerConfig,
		ControllerModelConfig: options.ControllerModelConfig,
	}
}

func expectedCommandBootstrapOptions(args bootstrap.RunBootstrapArgs) jujucommands.BootstrapOptions {
	return jujucommands.BootstrapOptions{
		BootstrapBase:        args.BootstrapOptions.BootstrapBase,
		BootstrapConstraints: args.BootstrapOptions.BootstrapConstraints,
		ModelConstraints:     args.BootstrapOptions.ModelConstraints,
		ModelDefault:         args.BootstrapOptions.ModelDefault,
		StoragePool: &jujucommands.StoragePool{
			Name:       args.BootstrapOptions.StoragePool.Name,
			Type:       args.BootstrapOptions.StoragePool.Type,
			Attributes: args.BootstrapOptions.StoragePool.Attributes,
		},
		BootstrapConfig:       args.BootstrapOptions.BootstrapConfig,
		ControllerConfig:      args.BootstrapOptions.ControllerConfig,
		ControllerModelConfig: args.BootstrapOptions.ControllerModelConfig,
	}
}

type bootstrapMocks struct {
	store           *bootstrapmocks.MockStore
	jujuManager     *bootstrapmocks.MockJujuManager
	binaryStore     *bootstrapmocks.MockBinaryStore
	commandFactory  *bootstrapmocks.MockCommandFactory
	clientStore     *bootstrapmocks.MockClientStore
	executor        *bootstrapmocks.MockJujuCommands
	credentialStore *bootstrapmocks.MockCredentialStore
	jobQueue        *bootstrapmocks.MockJobQueue
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
		jobQueue:        bootstrapmocks.NewMockJobQueue(ctrl),
	}

	i, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	user := openfga.NewUser(i, nil)

	return ctrl, m, user
}

func (s *bootstrapManagerSuite) Init(c *qt.C) {
	i, err := dbmodel.NewIdentity("admin@canonical.com")
	c.Assert(err, qt.IsNil)
	s.adminUser = openfga.NewUser(i, nil)
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLNotConfigured(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()
	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, "", mocks.credentialStore)
	c.Assert(err, qt.IsNil)
	_, err = manager.StartBootstrapJob(ctx, user, bootstrap.BootstrapParams{})
	c.Assert(err, qt.ErrorMatches, "bootstrap login token refresh URL is not configured.*")
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLEmpty(c *qt.C) {
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()
	_, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, "", mocks.credentialStore)
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_LoginTokenRefreshURLMalformed(c *qt.C) {
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()
	_, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, ":/url/", mocks.credentialStore)
	c.Assert(err, qt.ErrorMatches, ".*failed to parse bootstrap login token refresh URL.*")
}

func (s *bootstrapManagerSuite) TestStartBootstrapJob_EnqueuesJob(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	requested := bootstrap.BootstrapParams{
		CLIVersion:         "3.6.9",
		CloudNameAndRegion: "special-cloud",
		ControllerName:     "a",
		CloudCred:          jujucloud.Credential{},
		Cloud:              jujucloud.Cloud{},
		BootstrapOptions:   defaultBootstrapOptions(),
	}

	mocks.jobQueue.EXPECT().EnqueueBootstrap(gomock.Any(), rivertypes.BootstrapArgs{
		Username:             "bob@canonical.com",
		CLIVersion:           requested.CLIVersion,
		AgentVersion:         requested.CLIVersion,
		CloudNameAndRegion:   requested.CloudNameAndRegion,
		ControllerName:       requested.ControllerName,
		CloudCred:            requested.CloudCred,
		Cloud:                requested.Cloud,
		LoginTokenRefreshURL: loginTokenRefreshURLParam,
		BootstrapOptions:     expectedRiverBootstrapOptions(requested.BootstrapOptions),
	}).Return(&rivertype.JobInsertResult{
		Job: &rivertype.JobRow{
			ID: 99,
		},
	}, nil)

	jobID, err := manager.StartBootstrapJob(ctx, user, requested)
	c.Assert(err, qt.IsNil)
	c.Assert(jobID, qt.Equals, int64(99))
}

func (s *bootstrapManagerSuite) TestGetJobInfo(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := defaultJobID
	mocks.jobQueue.EXPECT().GetJobInfo(gomock.Any(), jobID).Return(&rivertype.JobRow{State: rivertype.JobStateRunning}, nil).AnyTimes()
	logsByJob := map[int64][]string{}
	mocks.store.EXPECT().QueryJobLog(gomock.Any(), jobID, gomock.Any()).DoAndReturn(
		func(_ context.Context, gotJobID int64, offset int) ([]string, int, error) {
			logs := logsByJob[gotJobID]
			if offset < 0 || offset > len(logs) {
				return nil, len(logs), nil
			}
			return append([]string{}, logs[offset:]...), len(logs), nil
		},
	).AnyTimes()

	numLogs := 101
	batchSize := 10
	watermark := 0
	for batch := 0; batch < numLogs/batchSize+1; batch++ {
		for j := 0; j < int(math.Min(float64(batchSize), float64(numLogs-batch*batchSize))); j++ {
			logsByJob[jobID] = append(logsByJob[jobID], "bootstrap logs "+fmt.Sprint(rune(batch*batchSize+j)))
		}

		response, err := manager.GetJobInfo(ctx, s.adminUser, jobID, watermark)
		c.Assert(err, qt.IsNil)
		logs := []string{}
		for j := 0; j < int(math.Min(float64(batchSize), float64(numLogs-batch*batchSize))); j++ {
			logs = append(logs, "bootstrap logs "+fmt.Sprint(rune(batch*batchSize+j)))
		}
		c.Check(response.Logs, qt.DeepEquals, logs)
		c.Assert(response.Status, qt.Equals, params.StatusRunning)
		watermark = response.Watermark
	}

	// check last batch is empty.
	response, err := manager.GetJobInfo(ctx, s.adminUser, jobID, watermark)
	c.Assert(response.Status, qt.Equals, params.StatusRunning)
	c.Assert(err, qt.IsNil)
	c.Assert(response.Logs, qt.HasLen, 0)
}

func (s *bootstrapManagerSuite) TestGetJobInfo_JobFailed(c *qt.C) {
	ctx := c.Context()
	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := defaultJobID
	mocks.store.EXPECT().QueryJobLog(gomock.Any(), jobID, gomock.Any()).Return(nil, 0, nil)
	mocks.jobQueue.EXPECT().GetJobInfo(gomock.Any(), jobID).Return(&rivertype.JobRow{
		State:  rivertype.JobStateDiscarded,
		Errors: []rivertype.AttemptError{{Error: "I died really fast"}},
	}, nil)

	response, err := manager.GetJobInfo(ctx, s.adminUser, jobID, 0)
	c.Assert(err, qt.IsNil)
	c.Assert(response.Status, qt.Equals, params.StatusFailed)
	c.Assert(response.Error, qt.Equals, "attempt 0: I died really fast\n")
}

func (s *bootstrapManagerSuite) TestGetJobInfo_JobNotFound(c *qt.C) {
	ctx := c.Context()
	jobID := int64(999)

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	mocks.jobQueue.EXPECT().GetJobInfo(gomock.Any(), jobID).Return(nil, errors.E("not found"))

	_, err = manager.GetJobInfo(ctx, s.adminUser, jobID, 0)
	c.Assert(err, qt.ErrorMatches, "failed to get job info: .*")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_Success(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := int64(1)
	mocks.jobQueue.EXPECT().WaitForJobCompletion(gomock.Any(), jobID).Return(&rivertype.JobRow{State: rivertype.JobStateCompleted}, nil)

	err = manager.WaitForJobCompletion(ctx, jobID, bootstrap.WaitConfig{})
	c.Assert(err, qt.IsNil)
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_JobFails(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := int64(2)
	mocks.jobQueue.EXPECT().WaitForJobCompletion(gomock.Any(), jobID).Return(&rivertype.JobRow{
		State:  rivertype.JobStateDiscarded,
		Errors: []rivertype.AttemptError{{Error: "job execution failed"}},
	}, nil)

	err = manager.WaitForJobCompletion(ctx, jobID, bootstrap.WaitConfig{})
	c.Assert(err, qt.ErrorMatches, ".*bootstrap job failed: job execution failed")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_JobNotFound(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := int64(3)
	mocks.jobQueue.EXPECT().WaitForJobCompletion(gomock.Any(), jobID).Return(nil, errors.E("not found"))

	err = manager.WaitForJobCompletion(ctx, jobID, bootstrap.WaitConfig{})
	c.Assert(err, qt.ErrorMatches, ".*error while waiting for job completion: not found")
}

func (s *bootstrapManagerSuite) TestWaitForJobCompletion_Timeout(c *qt.C) {
	ctx := c.Context()

	ctrl, mocks, _ := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := int64(4)
	mocks.jobQueue.EXPECT().WaitForJobCompletion(gomock.Any(), jobID).DoAndReturn(func(ctx context.Context, _ int64) (*rivertype.JobRow, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	// Wait with a very short timeout and fast polling
	err = manager.WaitForJobCompletion(ctx, jobID, bootstrap.WaitConfig{
		MaxJobDuration: 100 * time.Millisecond,
	})
	c.Assert(err, qt.ErrorMatches, ".*error while waiting for job completion: context deadline exceeded")
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

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	p := defaultBootstrapArgs()

	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
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
	mocks.clientStore.EXPECT().ControllerByName(p.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(p.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	for i := range hps {
		if hps[i].Scope == network.ScopeUnknown {
			hps[i].Scope = network.ScopePublic
		}
	}
	mocks.jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          p.ControllerName,
			PublicAddress: ctrlDetails.PublicDNSName,
			CACertificate: ctrlDetails.CACert,
			Addresses:     dbmodel.HostPorts{jujuparams.FromProviderHostPorts(hps)},
			TLSHostname:   "juju-apiserver",
		},
		gomock.Any(),
	).Return(nil)

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.IsNil)
	c.Assert(cleanupCalled, qt.IsTrue)
	// Check binary is no longer referenced.
	c.Assert(binaryDoneCalled, qt.Equals, true)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ControllerExists(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(nil)

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, `controller "a" already exists`)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ControllerRetrievalFails(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(errors.E("oh noes, we couldnt'se get the controller"))

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "failed to check if controller exists: oh noes, we couldnt'se get the controller")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_BinaryStoreGetFails(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
			Os:      runtime.GOOS,
			Arch:    runtime.GOARCH,
		},
		gomock.Any(),
	).Return(
		nil,
		errors.E("test error"),
	)

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "failed to get Juju binary: test error")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ExecutorFails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
		},
	).Return(
		func() chan jujucommands.OutputLine {
			ch := make(chan jujucommands.OutputLine)
			close(ch)
			return ch
		}(),
		mocks.clientStore,
		func() {},
		errors.E("executor test error"),
	)
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
		Return(mocks.executor)

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "run bootstrap failed: failed to run bootstrap command: executor test error")
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ReturnsEarlyIfLineErrors(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLineError := "command exited code 1"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
		Return(mocks.executor)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "run bootstrap failed: command failed: command exited code 1")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_ClientStoreFailsToGetControllerDetails(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
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
	mocks.clientStore.EXPECT().ControllerByName(p.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(p.ControllerName).Return(
		nil,
		errors.E("client store failed to get account details"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: p.ControllerName,
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

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches,
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

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
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
	mocks.clientStore.EXPECT().ControllerByName(p.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(p.ControllerName).Return(
		nil,
		errors.E("account details test error"),
	)
	mocks.executor.EXPECT().DestroyController(
		gomock.Any(),
		jujucommands.DestroyControllerCmdParams{
			ControllerName: p.ControllerName,
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

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches,
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

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
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
	mocks.clientStore.EXPECT().ControllerByName(p.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(p.ControllerName).Return(
		&jujuclient.AccountDetails{
			User:     "diglett",
			Password: "diglett's password",
		},
		nil,
	)
	hps, err := network.ParseProviderHostPorts(ctrlDetails.APIEndpoints...)
	c.Assert(err, qt.IsNil)
	for i := range hps {
		if hps[i].Scope == network.ScopeUnknown {
			hps[i].Scope = network.ScopePublic
		}
	}
	mocks.jujuManager.EXPECT().AddController(
		gomock.Any(),
		user,
		&dbmodel.Controller{
			UUID:          ctrlDetails.ControllerUUID,
			Name:          p.ControllerName,
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
			ControllerName: p.ControllerName,
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

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches,
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

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()

	// Mocked in order of execution:
	cleanupCalled := false // To be asserted after job run - ensures cleanup was run.
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
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
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).
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
	mocks.clientStore.EXPECT().ControllerByName(p.ControllerName).Return(
		ctrlDetails,
		nil,
	)
	mocks.clientStore.EXPECT().AccountDetails(p.ControllerName).Return(
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
			ControllerName: p.ControllerName,
		},
	).Return(
		nil,
		errors.E("cleanup controller test failure"),
	)

	err = manager.BootstrapController(testCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "(?s)run bootstrap failed: error post-bootstrap: failed to add controller to JIMM: add controller test error.*automatic cleanup of the controller also failed: failed to run destroy-controller command: cleanup controller test failure.*Controller details:.*")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestBootstrapJob_CancelledJob(c *qt.C) {
	testCtx := c.Context()

	binaryPath := "/faketmp/juju"
	failedBootstrapErr := "failed-bootstrap"

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	p := defaultBootstrapArgs()
	jobCtx, cancel := context.WithCancel(testCtx)
	defer cancel()

	cleanupCalled := false
	mocks.store.EXPECT().GetController(
		gomock.Any(),
		&dbmodel.Controller{Name: p.ControllerName},
	).Return(
		errors.E(errors.CodeNotFound, errors.E("test err")),
	)
	mocks.binaryStore.EXPECT().Get(
		gomock.Any(),
		jujuclistore.JujuBinarySpec{
			Version: p.CLIVersion,
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
			CloudNameAndRegion:   p.CloudNameAndRegion,
			ControllerName:       p.ControllerName,
			AgentVersion:         p.AgentVersion,
			DefaultLoginTokenURL: p.LoginTokenRefreshURL,
			Cloud:                p.Cloud,
			CloudCred:            p.CloudCred,
			BootstrapOptions:     expectedCommandBootstrapOptions(p),
		},
	).DoAndReturn(func(ctx context.Context, _ jujucommands.BootstrapCmdParams) (<-chan jujucommands.OutputLine, jujuclient.ClientStore, func(), error) {
		cancel()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second * 5):
			c.Error("expected context to be cancelled")
		}

		outputCh := make(chan jujucommands.OutputLine, 1)
		outputCh <- jujucommands.OutputLine{Line: "ignored", Err: errors.E(failedBootstrapErr)}
		close(outputCh)
		return outputCh, mocks.clientStore, func() { cleanupCalled = true }, nil
	})
	mocks.commandFactory.EXPECT().New(binaryPath, p.JujuDataDir).Return(mocks.executor)
	mocks.store.EXPECT().AddJobLog(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ int64, logLine string) error {
			if strings.Contains(logLine, failedBootstrapErr) {
				c.Assert(ctx.Err(), qt.IsNil)
			}
			return nil
		},
	).AnyTimes()

	err = manager.BootstrapController(jobCtx, p, mocks.commandFactory, user)
	c.Assert(err, qt.ErrorMatches, "run bootstrap failed: command failed: failed-bootstrap")
	c.Assert(cleanupCalled, qt.IsTrue)
}

func (s *bootstrapManagerSuite) TestStartDestroyControllerJob(c *qt.C) {
	testCtx := c.Context()

	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	args := bootstrap.DestroyControllerParams{
		ControllerName: "controller-foo",
		AgentVersion:   "3.6.9",
		ControllerUUID: "some-uuid",
		CloudName:      "cloud-foo",
		CloudRegion:    "region-foo",
		APIEndpoints:   []string{"ep-1", "ep-2"},
		PublicAddress:  "public-address",
		CACertificate:  "ca-cert",
	}

	mocks.jobQueue.EXPECT().EnqueueDestroyController(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(ctx context.Context, dca rivertypes.DestroyControllerArgs) (*rivertype.JobInsertResult, error) {
		c.Check(dca.Username, qt.Equals, user.Name)
		c.Check(dca.ControllerName, qt.Equals, "controller-foo")
		c.Check(dca.AgentVersion, qt.Equals, "3.6.9")
		c.Check(dca.ControllerUUID, qt.Equals, "some-uuid")
		c.Check(dca.CloudName, qt.Equals, "cloud-foo")
		c.Check(dca.CloudRegion, qt.Equals, "region-foo")
		c.Check(dca.APIEndpoints, qt.DeepEquals, []string{"ep-1", "ep-2"})
		c.Check(dca.PublicAddress, qt.Equals, "public-address")
		c.Check(dca.CACertificate, qt.Equals, "ca-cert")
		return &rivertype.JobInsertResult{
			Job: &rivertype.JobRow{ID: 456},
		}, nil
	})

	jobID, err := manager.StartDestroyControllerJob(testCtx, user, args)
	c.Assert(err, qt.IsNil)
	c.Assert(jobID, qt.Equals, int64(456))
}

func (s *bootstrapManagerSuite) TestDestroyController(c *qt.C) {
	testCtx := c.Context()

	cliVersion := "3.6.9"
	binaryPath := "/faketmp/juju"
	testOutputLine := "test-line"
	controllerName := "moribund"

	binary := &jujuclistore.Binary{FullPath: binaryPath}
	ctrl, mocks, user := setupTest(c)
	defer ctrl.Finish()

	manager, err := bootstrap.NewBootstrapManager(mocks.store, mocks.jobQueue, mocks.jujuManager, mocks.binaryStore, loginTokenRefreshURLParam, mocks.credentialStore)
	c.Assert(err, qt.IsNil)

	jobID := int64(456)
	jujuDataDir := "/path/to/a/juju/data/dir"
	args := bootstrap.RunDestroyControllerArgs{
		RunnerArgs: bootstrap.RunnerArgs{
			JujuDataDir: jujuDataDir,
			JobID:       jobID,
		},
		DestroyControllerArgs: rivertypes.DestroyControllerArgs{
			Username:       user.Name,
			ControllerName: controllerName,
			AgentVersion:   cliVersion,
		},
	}

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

	mocks.commandFactory.EXPECT().New(binaryPath, jujuDataDir).Return(mocks.executor)
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

	mocks.jujuManager.EXPECT().RemoveController(gomock.Any(), user, controllerName, true)

	err = manager.DestroyController(testCtx, args, mocks.commandFactory, user)
	c.Assert(err, qt.IsNil)
}

//go:generate go tool mockgen -typed -destination=./mocks/store.go -package=mocks . Store
//go:generate go tool mockgen -typed -destination=./mocks/jujumanager.go -package=mocks . JujuManager
//go:generate go tool mockgen -typed -destination=./mocks/binarystore.go -package=mocks . BinaryStore
//go:generate go tool mockgen -typed -destination=./mocks/commandfactory.go -package=mocks . CommandFactory
//go:generate go tool mockgen -typed -destination=./mocks/jujucommands.go -package=mocks . JujuCommands
//go:generate go tool mockgen -typed -destination=./mocks/jujuclientstore.go -package=mocks github.com/juju/juju/jujuclient ClientStore
//go:generate go tool mockgen -typed -destination=./mocks/credentialstore.go -package=mocks . CredentialStore
//go:generate go tool mockgen -typed -destination=./mocks/queue.go -package=mocks . JobQueue
func TestBootstrapManager(t *testing.T) {
	qtsuite.Run(qt.New(t), &bootstrapManagerSuite{})
}
