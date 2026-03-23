// Copyright 2026 Canonical.

package river

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/cloud"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/jimm/bootstrap"
	"github.com/canonical/jimm/v3/internal/openfga"
	"github.com/canonical/jimm/v3/internal/rivertypes"
)

func TestBootstrapWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database, sqlDb := setupTestDB(c)

	// Prepare identity needed by bootstrapWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	bootstrapManager := NewMockBootstrapManager(ctrl)
	openfgaClient := &openfga.OFGAClient{}
	w, err := newBootstrapWorker(openfgaClient, database, bootstrapManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	var gotArgs bootstrap.RunBootstrapArgs
	bootstrapManager.EXPECT().
		BootstrapController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, p bootstrap.RunBootstrapArgs, _ bootstrap.CommandFactory, _ *openfga.User) error {
			gotArgs = p
			c.Assert(p.CloudCred.Attributes(), qt.DeepEquals, map[string]string{"attr-1": "val-1"})
			c.Assert(p.CloudCred.AuthType(), qt.Equals, cloud.AccessKeyAuthType)
			c.Assert(p.Cloud.Name, qt.Equals, "mycloud")
			c.Assert(p.Cloud.Type, qt.Equals, "openstack")
			c.Assert(p.Cloud.Regions, qt.DeepEquals, []cloud.Region{
				{Name: "region-1", Endpoint: "region-1-endpoint"},
			})
			c.Assert(p.Username, qt.Equals, u.Name)
			c.Assert(p.ControllerName, qt.Equals, "controller-name")
			c.Assert(p.CloudNameAndRegion, qt.Equals, "aws/us-east-1")
			c.Assert(p.CLIVersion, qt.Equals, "3.6.1")
			c.Assert(p.LoginTokenRefreshURL, qt.Equals, "https://jimm.example.com/refresh")
			c.Assert(p.BootstrapOptions.BootstrapBase, qt.Equals, "ubuntu@24.04")
			c.Assert(p.BootstrapOptions.BootstrapConfig["some"], qt.Equals, "value")
			c.Assert(p.BootstrapOptions.ControllerConfig["audit-log-enabled"], qt.Equals, "true")
			c.Assert(p.JujuDataDir, qt.Not(qt.Equals), "")
			st, err := os.Stat(p.JujuDataDir)
			c.Assert(err, qt.IsNil)
			c.Assert(st.IsDir(), qt.IsTrue)
			return nil
		})

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	defer func() { _ = tx.Rollback() }()

	result, err := testWorker.Work(c.Context(), c.TB, tx, rivertypes.BootstrapArgs{
		Username: u.Name,
		CloudCred: cloud.NewCredential(cloud.AccessKeyAuthType, map[string]string{
			"attr-1": "val-1",
		}),
		Cloud: cloud.Cloud{Name: "mycloud", Type: "openstack", Regions: []cloud.Region{
			{Name: "region-1", Endpoint: "region-1-endpoint"},
		}},
		CLIVersion:           "3.6.1",
		CloudNameAndRegion:   "aws/us-east-1",
		ControllerName:       "controller-name",
		AgentVersion:         "3.6.0",
		LoginTokenRefreshURL: "https://jimm.example.com/refresh",
		BootstrapOptions: rivertypes.BootstrapOptions{
			BootstrapBase:    "ubuntu@24.04",
			BootstrapConfig:  map[string]string{"some": "value"},
			ControllerConfig: map[string]string{"audit-log-enabled": "true"},
		},
	}, nil)

	c.Assert(err, qt.IsNil)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobCompleted)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(gotArgs.JobID, qt.Equals, result.Job.ID)

	// Verify that the temporary Juju data directory has been removed.
	_, err = os.Stat(gotArgs.JujuDataDir)
	c.Assert(os.IsNotExist(err), qt.IsTrue)
}

func TestBootstrapWorker_Error(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database, sqlDb := setupTestDB(c)

	// Prepare identity needed by bootstrapWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	bootstrapManager := NewMockBootstrapManager(ctrl)
	openfgaClient := &openfga.OFGAClient{}
	w, err := newBootstrapWorker(openfgaClient, database, bootstrapManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	bootstrapManager.EXPECT().
		BootstrapController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("some-error"))

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	defer func() { _ = tx.Rollback() }()

	result, err := testWorker.Work(c.Context(), c.TB, tx, rivertypes.BootstrapArgs{
		Username:       u.Name,
		ControllerName: "controller-name",
	}, nil)
	c.Assert(err, qt.ErrorMatches, "some-error")
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateDiscarded)
}

func TestBootstrapWorker_Unique(t *testing.T) {
	c := qt.New(t)
	ctx, cancel := context.WithTimeout(c.Context(), 10*time.Second)
	c.Cleanup(cancel)

	testDeps := setupIntegrationTest(
		c,
		setupWorkerParams{},
	)
	bootstrapManager := testDeps.mockBootstrapManager
	riverClient := testDeps.riverClient
	username := testDeps.identity

	block := make(chan struct{})

	// First bootstrap blocks (keeps job running), second completes.
	gomock.InOrder(
		bootstrapManager.EXPECT().
			BootstrapController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, p bootstrap.RunBootstrapArgs, _ bootstrap.CommandFactory, _ *openfga.User) error {
				<-block
				return nil
			}),
		bootstrapManager.EXPECT().
			BootstrapController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil),
	)

	sub, cancelSub := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancelSub)

	ins1, err := riverClient.Insert(ctx, rivertypes.BootstrapArgs{
		Username:       username,
		ControllerName: "controller-name-1",
	}, nil)
	c.Assert(err, qt.IsNil)

	bootstrapKind := rivertypes.BootstrapArgs{}.Kind()
	waitForJobState(c, ctx, riverClient, ins1.Job.ID, rivertype.JobStateRunning, bootstrapKind)

	ins2, err := riverClient.Insert(ctx, rivertypes.BootstrapArgs{
		Username:       username,
		ControllerName: "controller-name-2",
	}, nil)
	c.Assert(err, qt.IsNil)
	// Because BootstrapArgs.InsertOpts uses only ByState uniqueness, the second insert should be skipped
	// while the first job is not in a completed state.
	c.Assert(ins2.UniqueSkippedAsDuplicate, qt.IsTrue)
	c.Assert(ins2.Job.ID, qt.Equals, ins1.Job.ID)

	close(block)
	row1 := waitForFinalisedJob(c, ctx, sub, ins1.Job.ID)
	c.Assert(row1.State, qt.Equals, rivertype.JobStateCompleted)

	// After the first job completes, a subsequent insert should not be considered a duplicate.
	ins3, err := riverClient.Insert(ctx, rivertypes.BootstrapArgs{
		Username:       username,
		ControllerName: "controller-name-3",
	}, nil)
	c.Assert(err, qt.IsNil)
	c.Assert(ins3.UniqueSkippedAsDuplicate, qt.IsFalse)
	c.Assert(ins3.Job.ID, qt.Not(qt.Equals), ins1.Job.ID)

	row3 := waitForFinalisedJob(c, ctx, sub, ins3.Job.ID)
	c.Assert(row3.State, qt.Equals, rivertype.JobStateCompleted)
}
