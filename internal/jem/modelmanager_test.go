// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/juju/clock/testclock"
	modelmanagerapi "github.com/juju/juju/api/modelmanager"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
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

func (s *modelManagerSuite) TestDestroyModel(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Sanity check the model exists
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{
		names.NewModelTag(model.UUID),
	})
	c.Assert(err, gc.Equals, nil)

	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is dying.
	m := mongodoc.Model{Path: model.Path}
	err = s.jem.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "dying")

	// Check that it can be destroyed twice.
	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, gc.Equals, nil)

	// Check the model is still dying.
	err = s.jem.DB.GetModel(testContext, &m)
	c.Assert(err, gc.Equals, nil)
	c.Assert(m.Life(), gc.Equals, "dying")
}

func (s *modelManagerSuite) TestDestroyModelWithStorage(c *gc.C) {
	model := s.bootstrapModel(c, params.EntityPath{User: "bob", Name: "model"})
	conn, err := s.jem.OpenAPI(testContext, model.Controller)
	c.Assert(err, gc.Equals, nil)
	defer conn.Close()

	// Sanity check the model exists
	tag := names.NewModelTag(model.UUID)
	client := modelmanagerapi.NewClient(conn)
	_, err = client.ModelInfo([]names.ModelTag{tag})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	f.MakeUnit(c, &factory.UnitParams{
		Application: f.MakeApplication(c, &factory.ApplicationParams{
			Charm: f.MakeCharm(c, &factory.CharmParams{
				Name: "storage-block",
			}),
			Storage: map[string]state.StorageConstraints{
				"data": {Pool: "modelscoped"},
			},
		}),
	})

	err = s.jem.DestroyModel(testContext, jemtest.NewIdentity("bob"), model, nil, nil, nil)
	c.Assert(err, jc.Satisfies, jujuparams.IsCodeHasPersistentStorage)
}

func (s *modelManagerSuite) bootstrapModel(c *gc.C, path params.EntityPath) *mongodoc.Model {
	return bootstrapModel(c, path, s.APIInfo(c), s.jem)
}
