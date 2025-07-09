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

	cl, cred, ctl, u := initModelEnv(c, db)

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Name:            "test-1",
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&model).Error, qt.IsNil)

	m := dbmodel.UserMapping{
		ModelUUID:        model.UUID,
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	m2 := dbmodel.UserMapping{
		ModelUUID:        model.UUID,
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m2).Error, qt.ErrorMatches, ".*duplicate key value violates unique constraint \"unique_user_mappings_key\".*")
}

func TestUserMappingModelUUIDNotEmpty(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl, cred, ctl, u := initModelEnv(c, db)

	// Create a model without a model UUID,
	// e.g. we are waiting for Juju to provide us the UUID.
	model := dbmodel.Model{
		Owner:           u,
		Name:            "test-1",
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&model).Error, qt.IsNil)

	m := dbmodel.UserMapping{
		ModelUUID:        model.UUID,
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m).Error, qt.IsNotNil)
}

func TestUserMappingIsDeletedWhenModelIsDeleted(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	cl, cred, ctl, u := initModelEnv(c, db)

	model := dbmodel.Model{
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Name:            "test-1",
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
	}
	c.Assert(db.Create(&model).Error, qt.IsNil)

	m := dbmodel.UserMapping{
		ModelUUID:        model.UUID,
		LocalUser:        "bob",
		ExternalUserName: u.Name,
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	var count int64
	c.Assert(db.Model(&dbmodel.UserMapping{}).Where("model_uuid = ?", model.UUID).Count(&count).Error, qt.IsNil)
	c.Assert(count, qt.Equals, int64(1))

	c.Assert(db.Delete(&model).Error, qt.IsNil)

	c.Assert(db.Model(&dbmodel.UserMapping{}).Where("model_uuid = ?", model.UUID).Count(&count).Error, qt.IsNil)
	c.Assert(count, qt.Equals, int64(0))
}
