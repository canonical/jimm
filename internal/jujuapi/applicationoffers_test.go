// Copyright 2020 Canonical Ltd.

package jujuapi_test

import (
	"context"

	"github.com/juju/charm/v7"
	"github.com/juju/juju/api/applicationoffers"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v2/bakery/identchecker"

	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
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

func (s *applicationOffersSuite) TestListApplicationOffers(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "user1", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "user1", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"user1"},
	})

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "user1", cred: cred})
	modelUUID := mi.UUID
	modelName := mi.Name
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

	results, err := client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	// without filters
	offers, err := client.ListOffers()
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "user1@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "everyone@external",
			DisplayName: "everyone",
			Access:      "read",
		}, {
			UserName:    "user1@external",
			DisplayName: "user1",
			Access:      "admin",
		}},
	}, {
		OfferName:              "test-offer2",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 2 description",
		OfferURL:               "user1@external/model-1.test-offer2",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "everyone@external",
			DisplayName: "everyone",
			Access:      "read",
		}, {
			UserName:    "user1@external",
			DisplayName: "user1",
			Access:      "admin",
		}},
	}})

	offers, err = client.ListOffers(crossmodel.ApplicationOfferFilter{
		ModelName:       modelName,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "user1@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "everyone@external",
			DisplayName: "everyone",
			Access:      "read",
		}, {
			UserName:    "user1@external",
			DisplayName: "user1",
			Access:      "admin",
		}},
	}})
}

func (s *applicationOffersSuite) TestModifyOfferAccess(c *gc.C) {
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

	results, err := client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.IsNil)

	offerURL := "user1@external/model-1.test-offer1"

	err = client.RevokeOffer(identchecker.Everyone+"@external", "read", offerURL)
	c.Assert(err, jc.ErrorIsNil)

	err = client.GrantOffer("test-user", "unknown", offerURL)
	err = client.GrantOffer("test-user@external", "unknown", offerURL)
	c.Assert(err, gc.ErrorMatches, `"unknown" offer access not valid`)

	err = client.GrantOffer("test-user@external", "read", "no-such-offer")
	c.Assert(err, gc.ErrorMatches, `not found`)

	err = client.GrantOffer("test-user@external", "admin", offerURL)
	c.Assert(err, jc.ErrorIsNil)

	offer := mongodoc.ApplicationOffer{
		OfferURL: offerURL,
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer)
	c.Assert(err, jc.ErrorIsNil)

	accessLevel, err := s.JEM.DB.GetApplicationOfferAccess(ctx, params.User("test-user"), offer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accessLevel, gc.Equals, mongodoc.ApplicationOfferAdminAccess)

	err = client.RevokeOffer("test-user@external", "consume", offerURL)
	c.Assert(err, jc.ErrorIsNil)

	accessLevel, err = s.JEM.DB.GetApplicationOfferAccess(ctx, params.User("test-user"), offer.OfferUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(accessLevel, gc.Equals, mongodoc.ApplicationOfferReadAccess)

	conn3 := s.open(c, nil, "user3")
	defer conn3.Close()
	client3 := applicationoffers.NewClient(conn3)

	err = client3.RevokeOffer("test-user@external", "read", offerURL)
	c.Assert(err, gc.ErrorMatches, "not found")
}

func (s *applicationOffersSuite) TestDestroyOffers(c *gc.C) {
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

	results, err := client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	offerURL := "user1@external/model-1.test-offer1"

	// user2 will have read access
	err = client.GrantOffer("user2@external", "read", offerURL)
	c.Assert(err, jc.ErrorIsNil)

	// try to destroy offer that does not exist
	err = client.DestroyOffers(true, "user1@external/model-1.test-offer2")
	c.Assert(err, gc.ErrorMatches, "not found")

	conn2 := s.open(c, nil, "user2")
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	// user2 is not authorized to destroy the offer
	err = client2.DestroyOffers(true, offerURL)
	c.Assert(err, gc.ErrorMatches, "unauthorized")

	// user1 can destroy the offer
	err = client.DestroyOffers(true, offerURL)
	c.Assert(err, jc.ErrorIsNil)

	offers, err := client.ListOffers(crossmodel.ApplicationOfferFilter{
		ModelName: mi.Name,
		OfferName: "test-offer1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationOffersSuite) TestFindApplicationOffers(c *gc.C) {
	ctx := context.Background()

	ctlPath := s.AssertAddController(ctx, c, params.EntityPath{User: "user1", Name: "controller-1"}, true)
	cred := s.AssertUpdateCredential(ctx, c, "user1", "dummy", "cred1", "empty")
	err := s.JEM.DB.SetACL(ctx, s.JEM.DB.Controllers(), ctlPath, params.ACL{
		Read: []string{"user1"},
	})

	mi := s.assertCreateModel(c, createModelParams{name: "model-1", username: "user1", cred: cred})
	modelUUID := mi.UUID
	modelName := mi.Name
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

	results, err := client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		modelUUID,
		"test-app",
		[]string{ep.Name},
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	// without filters
	offers, err := client.FindApplicationOffers()
	c.Assert(err, gc.ErrorMatches, "at least one filter must be specified")

	offers, err = client.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		ModelName:       modelName,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "user1@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "everyone@external",
			DisplayName: "everyone",
			Access:      "read",
		}, {
			UserName:    "user1@external",
			DisplayName: "user1",
			Access:      "admin",
		}},
	}})

	// by default each offer is publicly readable -> user2 should be
	// able to find it
	conn2 := s.open(c, nil, "user2")
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	offers, err = client2.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		ModelName:       modelName,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "user1@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName:    "everyone@external",
			DisplayName: "everyone",
			Access:      "read",
		}},
	}})
}
