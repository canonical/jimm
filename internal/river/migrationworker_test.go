package river

import (
	"errors"
	"testing"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"
)

func TestMigrationWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	upgradeManager := NewMockUpgradeManager(ctrl)

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	openfgaClient := &openfga.OFGAClient{}
	w, err := newMigrationWorker(openfgaClient, db, upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = db.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "test-uuid", "target-controller").
		Return(nil)

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(
		c.Context(),
		c.TB,
		tx,
		migrationWorkerArgs{
			Username:             u.Name,
			UUID:                 "test-uuid",
			TargetControllerName: "target-controller",
		},
		nil,
	)

	c.Assert(err, qt.IsNil)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobCompleted)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateCompleted)
}

func TestMigrationWorker_Error(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	upgradeManager := NewMockUpgradeManager(ctrl)

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	openfgaClient := &openfga.OFGAClient{}
	w, err := newMigrationWorker(openfgaClient, db, upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	u, err := dbmodel.NewIdentity("ash@catchum.com")
	c.Assert(err, qt.IsNil)
	err = db.GetIdentity(c.Context(), u)
	c.Assert(err, qt.IsNil)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), gomock.Any(), "test-uuid", "target-controller").
		Return(errors.New("oh noes"))

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(
		c.Context(),
		c.TB,
		tx,
		migrationWorkerArgs{
			Username:             u.Name,
			UUID:                 "test-uuid",
			TargetControllerName: "target-controller",
		},
		nil,
	)

	c.Assert(err, qt.ErrorMatches, "oh noes")
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateAvailable)
}
