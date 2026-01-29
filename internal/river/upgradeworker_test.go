// Copyright 2026 Canonical.

package river

import (
	"errors"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/version/v2"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"
	gomock "go.uber.org/mock/gomock"
)

func TestUpgradeWorker(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)
	w, err := newUpgradeWorker(upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), "test-string", version.Number{Major: 2, Minor: 0, Patch: 0}).
		Return(nil)

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	result, err := testWorker.Work(c.Context(), c.TB, tx, upgradeWorkerArgs{
		ModelUUID: "test-string", TargetVersion: version.MustParse("2.0.0")}, nil)
	c.Assert(err, qt.IsNil)
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobCompleted)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateCompleted)
}

func TestUpgradeWorker_Error(t *testing.T) {
	c := qt.New(t)

	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	db := setupTestDB(c)
	sqlDb, err := db.SqlDB()
	c.Assert(err, qt.IsNil)

	upgradeManager := NewMockUpgradeManager(ctrl)
	w, err := newUpgradeWorker(upgradeManager)
	c.Assert(err, qt.IsNil)

	testWorker := rivertest.NewWorker(c.TB, riverdatabasesql.New(sqlDb), nil, w)

	upgradeManager.EXPECT().
		UpgradeModel(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("some-error"))

	tx, err := sqlDb.Begin()
	c.Assert(err, qt.IsNil)

	result, err := testWorker.Work(c.Context(), c.TB, tx, upgradeWorkerArgs{
		ModelUUID: "test-string", TargetVersion: version.MustParse("2.0.0")}, nil)
	c.Assert(err, qt.ErrorMatches, "some-error")
	c.Assert(result.EventKind, qt.Equals, river.EventKindJobFailed)
	c.Assert(result.Job.State, qt.Equals, rivertype.JobStateAvailable)
}
