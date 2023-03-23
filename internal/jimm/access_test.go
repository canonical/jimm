// Copyright 2023 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
)

func TestJwtGenerator(t *testing.T) {
	c := qt.New(t)

	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
	c.Assert(err, qt.IsNil)

	j := &jimm.JIMM{
		UUID: uuid.NewString(),
		Database: db.Database{
			DB: jimmtest.MemoryDB(c, nil),
		},
		OpenFGAClient: client,
		JWTService:    nil,
	}
	ctx := context.Background()
	m := &dbmodel.Model{}
	authFunc := j.JwtGenerator(ctx, m)
	loginReq := new(jujuparams.LoginRequest)
	desiredPerms := map[string]interface{}{"model-<uuid>": "writer", "applicationoffer-<uuid>": "consumer"}
	token, err := authFunc(loginReq, desiredPerms)
	c.Assert(err, qt.IsNil)
	//Check token is valid and has correct assertions.
}
