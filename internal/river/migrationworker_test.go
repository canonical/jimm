package river_test

import (
	"errors"
	"testing"
	"time"

	"github.com/canonical/jimm/v3/internal/db"
	jimmriver "github.com/canonical/jimm/v3/internal/river"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
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

func TestUpgradeToWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)
	upgradeManager := NewMockUpgradeToManager(ctrl)
	w, err := jimmriver.NewUpgradeToWorker(upgradeManager)
	c.Assert(err, qt.IsNil)
	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)
	upgradeManager.EXPECT().
		UpgradeTo(gomock.Any(), gomock.Any(), "test-string", gomock.Any()).
		Return(version.Number{Major: 2, Minor: 0, Patch: 0}, nil)
	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(c.Context(), c.TB, tx, jimmriver.UpgradeToArgs{UUID: "test-string"}, nil)
	c.Assert(err, qt.IsNil)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobCompleted)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateCompleted)
}

func TestUpgradeToWorkerError(t *testing.T) {
	c := qt.New(t)
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)
	upgradeManager := NewMockUpgradeToManager(ctrl)
	w, err := jimmriver.NewUpgradeToWorker(upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)
	errUpgrade := errors.New("error upgrading")
	upgradeManager.EXPECT().
		UpgradeTo(gomock.Any(), gomock.Any(), "test-string", gomock.Any()).
		Return(version.Number{}, errUpgrade)
	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)
	result, err := testWorker.Work(c.Context(), c.TB, tx, jimmriver.UpgradeToArgs{UUID: "test-string"}, nil)
	c.Assert(err, qt.ErrorIs, errUpgrade)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateAvailable)
}
