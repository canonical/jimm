// Copyright 2025 Canonical.

package dbmodel_test

import (
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

// TestModelMigration_UniqueModelUUIDConstraint tests that the unique constraint
// on ModelMigration is enforced correctly, ensuring that two rows with the same
// ModelUUID cannot be created.
func TestModelMigration_UniqueModelUUIDConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	_, _, ctl, _ := initModelEnv(c, db)
	m := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		TargetControllerID: ctl.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)

	m2 := dbmodel.IncomingModelMigration{
		ModelUUID: sql.NullString{
			String: "00000001-0000-0000-0000-000000000001",
			Valid:  true,
		},
		TargetControllerID: ctl.ID,
		UserMapping:        dbmodel.StringMap{"local": "external"},
	}
	c.Assert(db.Create(&m2).Error, qt.ErrorMatches, ".*duplicate key value violates unique constraint \"incoming_model_migrations_model_uuid_key\".*")
}
