// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
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
		ID:               u.ID,
		CreatedAt:        u.CreatedAt,
		UpdatedAt:        u.UpdatedAt,
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
