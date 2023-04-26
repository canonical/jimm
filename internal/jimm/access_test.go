// Copyright 2023 Canonical Ltd.

package jimm_test

import (
	"context"
	"testing"
	"time"

	"github.com/CanonicalLtd/jimm/internal/db"
	"github.com/CanonicalLtd/jimm/internal/dbmodel"
	"github.com/CanonicalLtd/jimm/internal/jimm"
	"github.com/CanonicalLtd/jimm/internal/jimmtest"
	"github.com/CanonicalLtd/jimm/internal/openfga"
	ofganames "github.com/CanonicalLtd/jimm/internal/openfga/names"
	qt "github.com/frankban/quicktest"
	"github.com/google/uuid"
)

// TODO(Kian): We could use a test as below to unit test the
// auth function returned by JwtGenerator and ensure it correctly
// generates a JWT. The JWTGenerator requires the JIMM server to have
// a JWT service and cache setup, so we could either turn this into
// an interface and mock it or have the test start a full JIMM server.
// Also required an interface for authentication, mocked or Candid.
// This is already tested in an integration test in jujuapi/websocket_test.go
// func TestJwtGenerator(t *testing.T) {
// 	c := qt.New(t)

// 	_, client, _, err := jimmtest.SetupTestOFGAClient(c.Name())
// 	c.Assert(err, qt.IsNil)

// 	j := &jimm.JIMM{
// 		UUID: uuid.NewString(),
// 		Database: db.Database{
// 			DB: jimmtest.MemoryDB(c, nil),
// 		},
// 		OpenFGAClient: client,
// 		JWTService:    jimmjwx.NewJWTService(,),
// 	}
// 	ctx := context.Background()
// 	m := &dbmodel.Model{}
// 	authFunc := j.JwtGenerator(ctx, m)
// 	loginReq := new(jujuparams.LoginRequest)
// 	desiredPerms := map[string]interface{}{"model-123": "writer", "applicationoffer-123": "consumer"}
// 	token, err := authFunc(loginReq, desiredPerms)
// 	c.Assert(err, qt.IsNil)
// 	//Check token is valid and has correct assertions.
// }

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

	access := user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
	c.Assert(access, qt.Equals, ofganames.AuditLogViewerRelation)

	// re-granting access does not result in error.
	err = j.GrantAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	// admin user can revoke other users audit log access.
	err = j.RevokeAuditLogAccess(ctx, adminUser, user.ResourceTag())
	c.Assert(err, qt.IsNil)

	access = user.GetAuditLogViewerAccess(ctx, j.ResourceTag())
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
