// Copyright 2025 Canonical.

package dbmodel_test

import (
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

// TestUserMappingUniqueConstraint tests that the unique constraint on
// UserMapping is enforced correctly ensuring two rows with the same
// ModelUUID and LocalUser cannot be created.
func TestUserMappingUniqueConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(db.Create(&u).Error, qt.IsNil)

	m := dbmodel.UserMapping{
		ModelUUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	m2 := dbmodel.UserMapping{
		ModelUUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m2).Error, qt.ErrorMatches, ".*duplicate key value violates unique constraint \"unique_user_mappings_key\".*")
}
