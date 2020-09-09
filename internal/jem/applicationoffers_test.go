// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/juju/charm/v7"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"
	"gopkg.in/macaroon-bakery.v2/httpbakery"

	"github.com/CanonicalLtd/jimm/internal/auth"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jem"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/internal/pubsub"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationoffersSuite struct {
	jemtest.JujuConnSuite
	pool                           *jem.Pool
	sessionPool                    *mgosession.Pool
	jem                            *jem.JEM
	usageSenderAuthorizationClient *testUsageSenderAuthorizationClient
	model                          *mongodoc.Model
	endpoint                       state.Endpoint
	identity                       identchecker.ACLIdentity
	caCert                         string
	addrs                          []string

	suiteCleanups []func()
}

var _ = gc.Suite(&applicationoffersSuite{})

func (s *applicationoffersSuite) SetUpTest(c *gc.C) {
	ctx := context.Background()

	s.JujuConnSuite.SetUpTest(c)
	s.sessionPool = mgosession.NewPool(ctx, s.Session, 5)
	publicCloudMetadata, _, err := cloud.PublicCloudMetadata()
	c.Assert(err, gc.Equals, nil)
	s.usageSenderAuthorizationClient = &testUsageSenderAuthorizationClient{}
	s.PatchValue(&jem.NewUsageSenderAuthorizationClient, func(_ string, _ *httpbakery.Client) (jem.UsageSenderAuthorizationClient, error) {
		return s.usageSenderAuthorizationClient, nil
	})
	pool, err := jem.NewPool(ctx, jem.Params{
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
	s.jem = s.pool.JEM(ctx)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)

	info := s.APIInfo(c)
	s.addrs = info.Addrs
	hps, err := mongodoc.ParseAddresses(info.Addrs)
	c.Assert(err, gc.Equals, nil)
	s.caCert, _ = s.ControllerConfig.CACert()
	err = s.jem.DB.AddController(ctx, &mongodoc.Controller{
		Path: params.EntityPath{User: "user1", Name: "controller-1"},
		ACL: params.ACL{
			Read: []string{"everyone"},
		},
		CACert:        s.caCert,
		HostPorts:     [][]mongodoc.HostPort{hps},
		AdminUser:     info.Tag.Id(),
		AdminPassword: info.Password,
		UUID:          s.ControllerConfig.ControllerUUID(),
		Public:        true,
	})
	c.Assert(err, gc.Equals, nil)

	_, err = s.jem.UpdateCredential(ctx, &mongodoc.Credential{
		Path: mongodoc.CredentialPath{
			EntityPath: mongodoc.EntityPath{
				User: "user1",
				Name: "cred1",
			},
			Cloud: "dummy",
		},
		Type: "empty",
	}, 0)
	c.Assert(err, gc.Equals, nil)

	s.identity = jemtest.NewIdentity("user1")
	s.model, err = s.jem.CreateModel(auth.ContextWithIdentity(ctx, s.identity), jem.CreateModelParams{
		Path:           params.EntityPath{User: "user1", Name: "model-1"},
		ControllerPath: params.EntityPath{User: "user1", Name: "controller-1"},
		Credential: params.CredentialPath{
			Cloud: "dummy",
			User:  "user1",
			Name:  "cred1",
		},
		Cloud:  "dummy",
		Region: "dummy-region",
	})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(s.model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()

	f := factory.NewFactory(modelState.State, s.StatePool)
	app := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	s.endpoint, err = app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationoffersSuite) TearDownTest(c *gc.C) {
	s.jem.Close()
	s.pool.Close()
	s.sessionPool.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetails(c *gc.C) {
	ctx := context.Background()

	err := s.jem.Offer(ctx, s.identity, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offerURL := conv.ToOfferURL(s.model.Path, "test-offer")

	d := jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL,
		},
	}
	err = s.jem.GetApplicationOfferConsumeDetails(ctx, s.identity, &d, bakery.Version2)
	c.Assert(err, gc.Equals, nil)

	c.Check(d.Macaroon, gc.Not(gc.IsNil))
	d.Macaroon = nil
	c.Check(d.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	d.Offer.OfferUUID = ""
	c.Check(d, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               offerURL,
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "everyone@external",
				Access:   "read",
			}, {
				UserName: "user1@external",
				Access:   "admin",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Alias:         "controller-1",
			Addrs:         s.addrs,
			CACert:        s.caCert,
		},
	})
}
