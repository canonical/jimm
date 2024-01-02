// Copyright 2020 Canonical Ltd.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/internal/dbmodel"
)

func TestVersion(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	var v0 dbmodel.Version
	result := db.First(&v0, "component = ?", dbmodel.Component)
	c.Check(result.Error, qt.IsNil)
	c.Check(v0.Major, qt.DeepEquals, dbmodel.Major)
	c.Check(v0.Minor, qt.Equals, dbmodel.Minor)

	v1 := dbmodel.Version{
		Component: dbmodel.Component,
		Major:     dbmodel.Major,
		Minor:     dbmodel.Minor,
	}
	result = db.First(&v1, "component = ?", dbmodel.Component)
	c.Assert(result.Error, qt.IsNil)
	c.Check(v1, qt.DeepEquals, v1)

	v3 := dbmodel.Version{
		Component: dbmodel.Component,
		Major:     v1.Major + 1,
		Minor:     v1.Minor + 1,
	}
	result = db.Create(&v3)
	c.Check(result.Error, qt.ErrorMatches, `.*violates unique constraint "versions_pkey".*`)
	result = db.Save(&v3)
	c.Assert(result.Error, qt.IsNil)

	var v4 dbmodel.Version
	result = db.First(&v4, "component = ?", dbmodel.Component)
	c.Assert(result.Error, qt.IsNil)
	c.Check(v4, qt.DeepEquals, v3)
}
