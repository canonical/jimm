// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestAuthenticateNoAuthenticator(t *testing.T) {
	c := qt.New(t)

	j := &jimm.JIMM{}
	_, err := j.Authenticate(context.Background(), &jujuparams.LoginRequest{})
	c.Check(err, qt.ErrorMatches, `authenticator not configured`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeServerConfiguration)
}

func TestAuthenticate(t *testing.T) {
	c := qt.New(t)

	var auth jimmtest.Authenticator
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Authenticator: &auth,
	}
	ctx := context.Background()

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	auth.User = &dbmodel.User{
		Username:         "bob@external",
		DisplayName:      "Bob",
		ControllerAccess: "superuser",
	}
	u, err := j.Authenticate(ctx, nil)
	c.Assert(err, qt.IsNil)
	c.Check(u.Username, qt.Equals, "bob@external")
	c.Check(u.ControllerAccess, qt.Equals, "superuser")

	u2 := dbmodel.User{
		Username: "bob@external",
	}
	err = j.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)

	c.Check(u2, qt.DeepEquals, dbmodel.User{
		Model:            u.Model,
		Username:         "bob@external",
		DisplayName:      "Bob",
		ControllerAccess: "add-model",
		LastLogin: sql.NullTime{
			Time:  now,
			Valid: true,
		},
	})

	auth.Err = errors.E("test error", errors.CodeUnauthorized)
	u, err = j.Authenticate(ctx, nil)
	c.Check(err, qt.ErrorMatches, `test error`)
	c.Check(errors.ErrorCode(err), qt.Equals, errors.CodeUnauthorized)
	c.Check(u, qt.IsNil)
}

func TestAuditLogAccess(t *testing.T) {
	c := qt.New(t)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
	}
	ctx := context.Background()

	err := j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	adminUser := dbmodel.User{
		Username:         "alice",
		ControllerAccess: "superuser",
	}
	err = j.Database.GetUser(ctx, &adminUser)
	c.Assert(err, qt.Equals, nil)

	user := dbmodel.User{
		Username: "bob",
	}

	// admin user can grant other users audit log access (even if the
	// user does not exist yet).
	err = j.GrantAuditLogAccess(ctx, &adminUser, user.Tag().(names.UserTag))
	c.Assert(err, qt.Equals, nil)

	err = j.Database.GetUser(ctx, &user)
	c.Assert(err, qt.Equals, nil)
	c.Assert(user.AuditLogAccess, qt.Equals, "read")

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, &adminUser, user.Tag().(names.UserTag))
	c.Assert(err, qt.Equals, nil)

	err = j.Database.GetUser(ctx, &user)
	c.Assert(err, qt.Equals, nil)
	c.Assert(user.AuditLogAccess, qt.Equals, "")

	// non-admin user cannot grant/revoken audt log access
	err = j.GrantAuditLogAccess(ctx, &user, adminUser.Tag().(names.UserTag))
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, &user, adminUser.Tag().(names.UserTag))
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}
