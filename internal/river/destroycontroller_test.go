// Copyright 2026 Canonical.

package river

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
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

func TestDestroyControllerWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database, sqlDb := setupTestDB(c)

	// Prepare identity needed by destroyControllerWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	bootstrapManager := NewMockBootstrapManager(ctrl)
	openfgaClient := &openfga.OFGAClient{}
	w, err := newDestroyControllerWorker(openfgaClient, database, bootstrapManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	var gotArgs bootstrap.RunDestroyControllerArgs
	bootstrapManager.EXPECT().
		DestroyController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, p bootstrap.RunDestroyControllerArgs, _ bootstrap.CommandFactory, _ *openfga.User) error {
			gotArgs = p
			c.Assert(p.Username, qt.Equals, u.Name)
			c.Assert(p.ControllerName, qt.Equals, "controller-name")
			c.Assert(p.ControllerUUID, qt.Equals, "controller-uuid")
			c.Assert(p.AgentVersion, qt.Equals, "3.6.0")
			c.Assert(p.CloudName, qt.Equals, "aws")
			c.Assert(p.CloudRegion, qt.Equals, "us-east-1")
			c.Assert(p.APIEndpoints, qt.DeepEquals, []string{"10.0.0.1:17070", "10.0.0.2:17070"})
			c.Assert(p.PublicAddress, qt.Equals, "1.2.3.4")
			c.Assert(p.CACertificate, qt.Equals, "ca-cert")
			c.Assert(p.JujuDataDir, qt.Not(qt.Equals), "")
			st, err := os.Stat(p.JujuDataDir)
			c.Assert(err, qt.IsNil)
			c.Assert(st.IsDir(), qt.IsTrue)
			return nil
		})

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	defer func() { _ = tx.Rollback() }()

	result, err := testWorker.Work(c.Context(), c.TB, tx, rivertypes.DestroyControllerArgs{
		Username:       u.Name,
		ControllerName: "controller-name",
		ControllerUUID: "controller-uuid",
		AgentVersion:   "3.6.0",
		CloudName:      "aws",
		CloudRegion:    "us-east-1",
		APIEndpoints:   []string{"10.0.0.1:17070", "10.0.0.2:17070"},
		PublicAddress:  "1.2.3.4",
		CACertificate:  "ca-cert",
	}, nil)

	c.Assert(err, qt.IsNil)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobCompleted)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateCompleted)
	c.Assert(gotArgs.JobID, qt.Equals, result.Job.ID)

	// Verify that the temporary Juju data directory has been removed.
	_, err = os.Stat(gotArgs.JujuDataDir)
	c.Assert(os.IsNotExist(err), qt.IsTrue)
}

func TestDestroyControllerWorker_Error(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	database, sqlDb := setupTestDB(c)

	// Prepare identity needed by destroyControllerWorker.
	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = database.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	bootstrapManager := NewMockBootstrapManager(ctrl)
	openfgaClient := &openfga.OFGAClient{}
	w, err := newDestroyControllerWorker(openfgaClient, database, bootstrapManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	bootstrapManager.EXPECT().
		DestroyController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("some-error"))

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	defer func() { _ = tx.Rollback() }()

	result, err := testWorker.Work(c.Context(), c.TB, tx, rivertypes.DestroyControllerArgs{
		Username:       u.Name,
		ControllerName: "controller-name",
		ControllerUUID: "controller-uuid",
		AgentVersion:   "3.6.0",
	}, nil)

	c.Assert(err, qt.ErrorMatches, "some-error")
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateDiscarded)
}

func TestDestroyControllerWorker_Unique(t *testing.T) {
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

	// First destroy blocks (keeps job running), second completes.
	gomock.InOrder(
		bootstrapManager.EXPECT().
			DestroyController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, p bootstrap.RunDestroyControllerArgs, _ bootstrap.CommandFactory, _ *openfga.User) error {
				<-block
				return nil
			}),
		bootstrapManager.EXPECT().
			DestroyController(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil),
	)

	sub, cancelSub := riverClient.Subscribe(river.EventKindJobCompleted)
	c.Cleanup(cancelSub)

	ins1, err := riverClient.Insert(ctx, rivertypes.DestroyControllerArgs{
		Username:       username,
		ControllerName: "controller-name-1",
		ControllerUUID: "uuid-1",
		AgentVersion:   "3.6.0",
	}, nil)
	c.Assert(err, qt.IsNil)

	destroyKind := rivertypes.DestroyControllerArgs{}.Kind()
	waitForJobState(c, ctx, riverClient, ins1.Job.ID, rivertype.JobStateRunning, destroyKind)

	ins2, err := riverClient.Insert(ctx, rivertypes.DestroyControllerArgs{
		Username:       username,
		ControllerName: "controller-name-2",
		ControllerUUID: "uuid-2",
		AgentVersion:   "3.6.0",
	}, nil)
	c.Assert(err, qt.IsNil)
	// Because DestroyControllerArgs.InsertOpts uses only ByState uniqueness, the second insert should be skipped
	// while the first job is not in a completed state.
	c.Assert(ins2.UniqueSkippedAsDuplicate, qt.IsTrue)
	c.Assert(ins2.Job.ID, qt.Equals, ins1.Job.ID)

	close(block)
	row1 := waitForFinalisedJob(c, ctx, sub, ins1.Job.ID)
	c.Assert(row1.State, qt.Equals, rivertype.JobStateCompleted)

	// After the first job completes, a subsequent insert should not be considered a duplicate.
	ins3, err := riverClient.Insert(ctx, rivertypes.DestroyControllerArgs{
		Username:       username,
		ControllerName: "controller-name-3",
		ControllerUUID: "uuid-3",
		AgentVersion:   "3.6.0",
	}, nil)
	c.Assert(err, qt.IsNil)
	c.Assert(ins3.UniqueSkippedAsDuplicate, qt.IsFalse)
	c.Assert(ins3.Job.ID, qt.Not(qt.Equals), ins1.Job.ID)

	row3 := waitForFinalisedJob(c, ctx, sub, ins3.Job.ID)
	c.Assert(row3.State, qt.Equals, rivertype.JobStateCompleted)
}
