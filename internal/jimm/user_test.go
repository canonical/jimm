// Copyright 2021 Canonical Ltd.

package jimm_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	qt "github.com/frankban/quicktest"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v4"

	"github.com/canonical/jimm/internal/db"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/errors"
	"github.com/canonical/jimm/internal/jimm"
	"github.com/canonical/jimm/internal/jimmtest"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
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

	client, _, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	var auth jimmtest.Authenticator
	now := time.Now().UTC().Round(time.Millisecond)
	j := &jimm.JIMM{
		UUID: "test",
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, func() time.Time { return now }),
		},
		Authenticator: &auth,
		OpenFGAClient: client,
	}
	ctx := context.Background()

	err = j.Database.Migrate(ctx, false)
	c.Assert(err, qt.IsNil)

	auth.User = openfga.NewUser(
		&dbmodel.User{
			Username:    "bob@external",
			DisplayName: "Bob",
		},
		client,
	)
	u, err := j.Authenticate(ctx, nil)
	c.Assert(err, qt.IsNil)
	c.Check(u.Username, qt.Equals, "bob@external")
	c.Check(u.JimmAdmin, qt.IsFalse)

	err = auth.User.SetControllerAccess(
		context.Background(),
		names.NewControllerTag(j.UUID),
		ofganames.AdministratorRelation,
	)
	c.Assert(err, qt.IsNil)

	u, err = j.Authenticate(ctx, nil)
	c.Assert(err, qt.IsNil)
	c.Check(u.Username, qt.Equals, "bob@external")
	c.Check(u.JimmAdmin, qt.IsTrue)

	u2 := dbmodel.User{
		Username: "bob@external",
	}
	err = j.Database.GetUser(ctx, &u2)
	c.Assert(err, qt.IsNil)

	c.Check(u2, qt.DeepEquals, dbmodel.User{
		Model:       u.Model,
		Username:    "bob@external",
		DisplayName: "Bob",
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
