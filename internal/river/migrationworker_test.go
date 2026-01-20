package river_test

import (
	"errors"
	"testing"
	"time"

	"github.com/canonical/jimm/v3/internal/db"
	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	jimmriver "github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	qt "github.com/frankban/quicktest"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"
)

func setupTestDB(c *qt.C) *db.Database {
	db := &db.Database{
		DB: jimmtest.PostgresDB(c, time.Now),
	}
	err := jimmriver.MigrateRiver(c.Context(), db)
	c.Assert(err, qt.IsNil)
	return db
}

func TestMigrationWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	upgradeManager := NewMockMigrationWorkerUpgradeManager(ctrl)

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	w, err := jimmriver.NewUpgradeMigrationWorker(upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	u, err := dbmodel.NewIdentity("ash@catchum.com")
	ofgaUser := openfga.NewUser(u, nil)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), ofgaUser, "test-uuid", "target-controller").
		Return(nil)

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(
		c.Context(),
		c.TB,
		tx,
		jimmriver.UpgradeMigrationWorker{
			User:                 ofgaUser,
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

	upgradeManager := NewMockMigrationWorkerUpgradeManager(ctrl)

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	w, err := jimmriver.NewUpgradeMigrationWorker(upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	u, err := dbmodel.NewIdentity("ash@catchum.com")
	ofgaUser := openfga.NewUser(u, nil)

	upgradeManager.EXPECT().
		MigrateModel(gomock.Any(), ofgaUser, "test-uuid", "target-controller").
		Return(errors.New("oh noes"))

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(
		c.Context(),
		c.TB,
		tx,
		jimmriver.UpgradeMigrationWorker{
			User:                 ofgaUser,
			UUID:                 "test-uuid",
			TargetControllerName: "target-controller",
		},
		nil,
	)

	c.Assert(err, qt.ErrorMatches, "oh noes")
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateAvailable)
}
