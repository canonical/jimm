// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/errors"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	openfga "github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
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

	auth.User = &openfga.User{
		User: &dbmodel.User{
			Username:         "bob@external",
			DisplayName:      "Bob",
			ControllerAccess: "superuser",
		},
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
		ControllerAccess: "login",
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

	_, ofgaClient, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		OpenFGAClient: ofgaClient,
	}
	ctx := context.Background()

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	adminUser := openfga.NewUser(&dbmodel.User{Username: "alice"}, j.OpenFGAClient)
	err = adminUser.SetControllerAccess(ctx, j.ResourceTag(), ofganames.AdministratorRelation)
	c.Assert(err, qt.IsNil)

	user := openfga.NewUser(&dbmodel.User{Username: "bob"}, j.OpenFGAClient)

	// admin user can grant other users audit log access.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access := user.GetControllerAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.AuditLogViewerRelation)

	// re-granting access does not result in error.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access = user.GetControllerAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.NoRelation)

	// re-revoking access does not result in error.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// non-admin user cannot grant audit log access
	err = j.GrantAuditLogAccess(ctx, user, adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// non-admin user cannot revoke audit log access
	err = j.RevokeAuditLogAccess(ctx, user, adminUser.ResourceTag())
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}
