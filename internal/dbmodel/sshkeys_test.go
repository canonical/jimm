// Copyright 2025 Canonical.

package dbmodel_test

import (
	"database/sql"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/juju/state"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestSSHKeyUniqueConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)
	cl, cred, ctl, u := initModelEnv(c, db)
	m := dbmodel.Model{
		Name: "test-model",
		UUID: sql.NullString{
			String: "00000001-0000-0000-0000-0000-000000000001",
			Valid:  true,
		},
		Owner:           u,
		Controller:      ctl,
		CloudRegion:     cl.Regions[0],
		CloudCredential: cred,
		Life:            state.Alive.String(),
	}
	c.Assert(db.Create(&m).Error, qt.IsNil)
	key := dbmodel.SSHKey{
		PublicKey:  []byte("test"),
		Identity:   u,
		KeyComment: "foo",
		Model:      m,
	}
	c.Assert(db.Create(&key).Error, qt.IsNil)

	newKey := dbmodel.SSHKey{
		PublicKey:  []byte("test"),
		Identity:   u,
		KeyComment: "bar",
		Model:      m,
	}
	// here we expect an error, which will be ignored then at the db level.
	c.Assert(db.Create(&newKey).Error, qt.ErrorMatches, ".*duplicate key value violates unique constraint \"unique_identity_ssh_key\".*")
}
