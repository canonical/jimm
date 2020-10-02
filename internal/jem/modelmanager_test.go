// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/juju/clock/testclock"
	"github.com/juju/juju/cloud"
	"github.com/juju/names/v4"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/httpbakery"
	"gopkg.in/mgo.v2/bson"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type modelManagerSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
}

var _ = gc.Suite(&controllerSuite{})

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(context.TODO(), s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(context.TODO(), jem.Params{
		DB:                  s.Session.DB("jem"),
		ControllerAdmin:     "controller-admin",
		SessionPool:         s.sessionPool,
		PublicCloudMetadata: publicCloudMetadata,
		UsageSenderURL:      "test-usage-sender-url",
		Pubsub: &pubsub.Hub{
			MaxConcurrency: 10,
		},
	})
	c.Assert(err, gc.Equals, nil)
	s.pool = pool
	s.jem = s.pool.JEM(context.TODO())
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *modelManagerSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *modelManagerSuite) TestValidateModelUpgrade(c *gc.C) {
	now := bson.Now()
	s.PatchValue(jem.WallClock, testclock.NewClock(now))
	ctlId := addController(c, params.EntityPath{User: "bob", Name: "controller"}, s.APIInfo(c), s.jem)
	err := s.jem.DB.SetACL(testContext, s.jem.DB.Controllers(), ctlId, params.ACL{
		Read: []string{"everyone"},
	})
	c.Assert(err, gc.Equals, nil)
	// Bob has a single credential.
	err = jem.UpdateCredential(s.jem.DB, testContext, &mongodoc.Credential{
		Path: mgoCredentialPath("dummy", "bob", "cred1"),
		Type: "empty",
	})
	ctx := auth.ContextWithIdentity(testContext, jemtest.NewIdentity("bob"))
	model, err := s.jem.CreateModel(ctx, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "oldmodel"},
		ControllerPath: ctlId,
		Credential:     credentialPath("dummy", "bob", "cred1"),
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.ValidateModelUpgrade(ctx, jemtest.NewIdentity("alice"), names.NewModelTag(model.UUID).String(), true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	err = s.jem.ValidateModelUpgrade(ctx, jemtest.NewIdentity("bob"), names.NewModelTag(model.UUID).String(), false)
	c.Assert(err, gc.Equals, nil)

	err = s.jem.ValidateModelUpgrade(ctx, jemtest.NewIdentity("bob"), names.NewModelTag(utils.MustNewUUID().String()).String(), true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}
