// Copyright 2020 Canonical Ltd.

package jujuapi_test

import (
	"context"

	"github.com/CanonicalLtd/jimm/params"
	"github.com/juju/juju/api/applicationoffers"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
)

type applicationOffersSuite struct {
	websocketSuite
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.ServerParams.CharmstoreLocation = "https://api.jujucharms.com/charmstore"
	s.ServerParams.MeteringLocation = "https://api.jujucharms.com/omnibus"
	s.websocketSuite.SetUpTest(c)
	s.PatchValue(&utils.OutgoingAccessAllowed, true)
}

func (s *applicationOffersSuite) TestOffer(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "user1", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "user1", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"user1"},
	})

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "user1", cred: cred})
	modelUUID := mi.UUID
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "user1", Name: "model-1"}, params.ACL{
		Admin: []string{"user1"},
	})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(modelUUID)
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "user1")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(modelUUID, "test-app", []string{ep.Name}, "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(modelUUID, "no-such-app", []string{ep.Name}, "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Not(gc.IsNil))
	c.Assert(results[0].Error.Code, gc.Equals, "not found")

	conn1 := s.open(c, nil, "alice")
	defer conn1.Close()
	client1 := applicationoffers.NewClient(conn1)

	results, err = client1.Offer(modelUUID, "test-app", []string{ep.Name}, "test-offer-2", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error.Code, gc.Equals, "unauthorized access")
}

func (s *applicationOffersSuite) TestGetConsumeDetails(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "user1", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "user1", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"user1"},
	})

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "user1", cred: cred})
	modelUUID := mi.UUID
	err = s.JEM.DB.SetACL(ctx, s.JEM.DB.Models(), params.EntityPath{User: "user1", Name: "model-1"}, params.ACL{
		Admin: []string{"user1"},
	})
	c.Assert(err, gc.Equals, nil)

	modelState, err := s.StatePool.Get(modelUUID)
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	conn := s.open(c, nil, "user1")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(modelUUID, "test-app", []string{ep.Name}, "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	ourl := &crossmodel.OfferURL{
		User:            "user1@external",
		ModelName:       "model-1",
		ApplicationName: "test-offer",
	}

	details, err := client.GetConsumeDetails(ourl.Path())
	c.Assert(err, gc.Equals, nil)
	c.Check(details.Macaroon, gc.Not(gc.IsNil))
	details.Macaroon = nil
	c.Check(details.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	details.Offer.OfferUUID = ""
	caCert, _ := s.ControllerConfig.CACert()
	info := s.APIInfo(c)
	c.Check(details, gc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(modelUUID).String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
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
			Addrs:         info.Addrs,
			Alias:         "controller-1",
			CACert:        caCert,
		},
	})

	ourl2 := &crossmodel.OfferURL{
		ModelName:       "model-1",
		ApplicationName: "test-offer",
	}

	details, err = client.GetConsumeDetails(ourl2.Path())
	c.Assert(err, gc.Equals, nil)
	c.Check(details.Macaroon, gc.Not(gc.IsNil))
	details.Macaroon = nil
	c.Check(details.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	details.Offer.OfferUUID = ""
	c.Check(details, gc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(modelUUID).String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
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
			Addrs:         info.Addrs,
			Alias:         "controller-1",
			CACert:        caCert,
		},
	})
}
