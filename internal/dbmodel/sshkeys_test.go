// Copyright 2025 Canonical.

package dbmodel_test

import (
	"testing"

	qt "github.com/frankban/quicktest"

	"github.com/canonical/jimm/v3/internal/dbmodel"
)

func TestSSHKeyUniqueConstraint(t *testing.T) {
	c := qt.New(t)
	db := gormDB(c)

	u, err := dbmodel.NewIdentity("bob@canonical.com")
	c.Assert(err, qt.IsNil)

	c.Assert(db.Create(u).Error, qt.IsNil)

	key := dbmodel.SSHKey{
		PublicKey:  []byte("test"),
		Identity:   *u,
		KeyComment: "foo",
	}
	c.Assert(db.Create(&key).Error, qt.IsNil)

	newKey := dbmodel.SSHKey{
		PublicKey:  []byte("test"),
		Identity:   *u,
		KeyComment: "bar",
	}
	c.Assert(db.Create(&newKey).Error, qt.ErrorMatches, ".*duplicate key value violates unique constraint \"unique_identity_ssh_key\".*")
}
