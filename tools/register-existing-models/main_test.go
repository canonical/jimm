// Copyright 2017 Canonical Ltd.

package main_test

import (
	"encoding/json"

	jc "github.com/juju/testing/checkers"
	"golang.org/x/net/context"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/CanonicalLtd/jem/internal/apitest"
	"github.com/CanonicalLtd/jem/internal/auth"
	"github.com/CanonicalLtd/jem/internal/jem"
	"github.com/CanonicalLtd/jem/internal/mongodoc"
	"github.com/CanonicalLtd/jem/params"
	cmd "github.com/CanonicalLtd/jem/tools/register-existing-models"
)

var testContext = context.Background()

type modelRegisterSuite struct {
	apitest.Suite
}

var _ = gc.Suite(&modelRegisterSuite{})

func (s *modelRegisterSuite) TestModelRegistration(c *gc.C) {
	registrationClient := &testMetricsRegistrationClient{}
	s.PatchValue(&cmd.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (cmd.UsageSenderAuthorizationClient, error) {
		return registrationClient, nil
	})
	ctlId := s.AssertAddController(c, params.EntityPath{"bob", "foo"}, false)
	cred := s.AssertUpdateCredential(c, "bob", "dummy", "cred1", "empty")

	// this model will get usage sending credentials..
	s.CreateModel(c, params.EntityPath{"bob", "foo"}, ctlId, cred)

	// this model won't have usage sending credentials..
	ctx := auth.ContextWithUser(testContext, "bob")
	createdModel, err := s.JEM.CreateModel(ctx, jem.CreateModelParams{
		Path:           params.EntityPath{"bob", "oldmodel"},
		ControllerPath: ctlId,
		Credential: params.CredentialPath{
			Cloud:      "dummy",
			EntityPath: params.EntityPath{"bob", "cred1"},
		},
		Cloud: "dummy",
	})
	c.Assert(err, jc.ErrorIsNil)

	kp, err := bakery.GenerateKey()
	c.Assert(err, jc.ErrorIsNil)

	collection := s.Session.DB("jem").C("models")
	err = cmd.UpdateModels(ctx, collection, &cmd.Config{
		IdentityLocation: "http://test.com",
		UsageSenderURL:   "http://test.com",
		AgentUsername:    "jimm",
		AgentKey:         kp,
		JIMMPlan:         "canonical/jimm",
		JIMMCharm:        "cs:~canonical/jimm-0",
		JIMMOwner:        "canonical",
		JIMMName:         "jimm",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(registrationClient.calls, gc.Equals, 1)
	c.Assert(registrationClient.applicationUser, gc.Equals, "bob")

	data, err := json.Marshal(registrationClient.m)
	c.Assert(err, jc.ErrorIsNil)

	var updatedModel mongodoc.Model
	err = collection.FindId(createdModel.Id).One(&updatedModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(updatedModel.UsageSenderCredentials, jc.DeepEquals, data)
}

type testMetricsRegistrationClient struct {
	plan             string
	charm            string
	application      string
	applicationOwner string
	applicationUser  string
	m                *macaroon.Macaroon
	calls            int
}

func (c *testMetricsRegistrationClient) AuthorizeReseller(plan, charm, application, applicationOwner, applicationUser string) (*macaroon.Macaroon, error) {
	c.plan, c.charm, c.application, c.applicationOwner, c.applicationUser = plan, charm, application, applicationOwner, applicationUser
	c.calls++
	m, err := macaroon.New(nil, "", "jem")
	c.m = m
	return c.m, err
}
