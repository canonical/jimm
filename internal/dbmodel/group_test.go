// Copyright 2024 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestGroupEntry(t *testing.T) {
	c := qt.New(t)
	db := gormDB(t)

	ge := dbmodel.GroupEntry{
		Name: "test-group-1",
	}
	c.Assert(db.Create(&ge).Error, qt.IsNil)
	c.Assert(ge.ID, qt.Equals, uint(1))

	ge1 := dbmodel.GroupEntry{
		Name: "test-group-1",
	}
	c.Assert(db.Create(&ge1).Error, qt.ErrorMatches, `.*violates unique constraint "groups_name_key".*`)

	var ge2 dbmodel.GroupEntry
	c.Assert(db.First(&ge2).Error, qt.IsNil)
	c.Check(ge2, qt.DeepEquals, ge)

	ge3 := dbmodel.GroupEntry{
		Name: "test-group-1",
	}
	result := db.First(&ge3)
	c.Assert(result.Error, qt.IsNil)
	c.Assert(ge3, qt.DeepEquals, ge)
}
