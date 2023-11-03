// Copyright 2020 Canonical Ltd.
package jujuapi_test

import (
	"context"
	"sort"

	"github.com/juju/charm/v11"
	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/internal/auth"
	"github.com/canonical/jimm/internal/dbmodel"
	"github.com/canonical/jimm/internal/openfga"
	ofganames "github.com/canonical/jimm/internal/openfga/names"
)

type applicationOffersSuite struct {
	websocketSuite
	state    *state.PooledState
	factory  *factory.Factory
	endpoint state.Endpoint
}

var _ = gc.Suite(&applicationOffersSuite{})

func (s *applicationOffersSuite) SetUpTest(c *gc.C) {
	s.websocketSuite.SetUpTest(c)
	var err error
	s.state, err = s.StatePool.Get(s.Model.UUID.String)
	c.Assert(err, gc.Equals, nil)
	s.factory = factory.NewFactory(s.state.State, s.StatePool)
	app := s.factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app",
		Charm: s.factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	s.factory.MakeUnit(c, &factory.UnitParams{
		Application: app,
	})
	s.endpoint, err = app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationOffersSuite) TearDownTest(c *gc.C) {
	s.endpoint = state.Endpoint{}
	s.factory = nil
	if s.state != nil {
		s.state.Release()
		s.state = nil
	}
	s.websocketSuite.TearDownTest(c)
}

func (s *applicationOffersSuite) TestOffer(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(s.Model.UUID.String, "test-app", []string{s.endpoint.Name}, "bob@external", "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(s.Model.UUID.String, "no-such-app", []string{s.endpoint.Name}, "bob@external", "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Not(gc.IsNil))
	c.Assert(results[0].Error.Code, gc.Equals, "not found")

	conn1 := s.open(c, nil, "charlie@external")
	defer conn1.Close()
	client1 := applicationoffers.NewClient(conn1)

	results, err = client1.Offer(s.Model.UUID.String, "test-app", []string{s.endpoint.Name}, "bob@external", "test-offer-2", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error.Code, gc.Equals, "unauthorized access")
}

func (s *applicationOffersSuite) TestGetConsumeDetails(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(s.Model.UUID.String, "test-app", []string{s.endpoint.Name}, "bob@external", "test-offer", "test offer description")
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	ourl := &crossmodel.OfferURL{
		User:            "bob@external",
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

	sort.Slice(details.Offer.Users, func(i, j int) bool {
		return details.Offer.Users[i].UserName < details.Offer.Users[j].UserName
	})
	c.Check(details, gc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         s.Model.Tag().String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@external",
				Access:   "admin",
			}, {
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: auth.EveryoneUser,
				Access:   "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: s.Model.Controller.Tag().String(),
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
	sort.Slice(details.Offer.Users, func(j, k int) bool {
		return details.Offer.Users[j].UserName < details.Offer.Users[k].UserName
	})
	c.Check(details, gc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         s.Model.Tag().String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@external",
				Access:   "admin",
			}, {
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: auth.EveryoneUser,
				Access:   "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: s.Model.Controller.Tag().String(),
			Addrs:         info.Addrs,
			Alias:         "controller-1",
			CACert:        caCert,
		},
	})
}

func (s *applicationOffersSuite) TestListApplicationOffers(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	// without filters
	_, err = client.ListOffers()
	c.Assert(err, gc.ErrorMatches, `at least one filter must be specified \(bad request\)`)

	offers, err := client.ListOffers(crossmodel.ApplicationOfferFilter{
		ModelName:       s.Model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)

	for i, offer := range offers {
		// mask the charm URL as it changes depending on the test
		// run order.
		offer.CharmURL = ""
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@external",
			Access:   "admin",
		}, {
			UserName: "bob@external",
			Access:   "admin",
		}, {
			UserName: auth.EveryoneUser,
			Access:   "read",
		}},
	}})
}

func (s *applicationOffersSuite) TestModifyOfferAccess(c *gc.C) {
	/*
		ctx := context.Background()

		conn := s.open(c, nil, "bob@external")
		defer conn.Close()
		client := applicationoffers.NewClient(conn)

		results, err := client.Offer(
			s.Model.UUID.String,
			"test-app",
			[]string{s.endpoint.Name},
			"bob@external",
			"test-offer1",
			"test offer 1 description",
		)
		c.Assert(err, gc.Equals, nil)
		c.Assert(results, gc.HasLen, 1)
		c.Assert(results[0].Error, gc.IsNil)

		offerURL := "bob@external/model-1.test-offer1"

		err = client.RevokeOffer(auth.Everyone, "read", offerURL)
		c.Assert(err, jc.ErrorIsNil)

		err = client.GrantOffer("test.user@external", "unknown", offerURL)
		c.Assert(err, gc.ErrorMatches, `"unknown" offer access not valid`)

		err = client.GrantOffer("test.user@external", "read", "no-such-offer")
		c.Assert(err, gc.ErrorMatches, `application offer not found`)

		err = client.GrantOffer("test.user@external", "admin", offerURL)
		c.Assert(err, jc.ErrorIsNil)

		testUser := dbmodel.User{
			Username: "test.user@external",
		}

		offer := dbmodel.ApplicationOffer{
			URL: offerURL,
		}
		err = s.JIMM.Database.GetApplicationOffer(ctx, &offer)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(offer.UserAccess(&testUser), gc.Equals, "admin")

		err = client.RevokeOffer("test.user@external", "consume", offerURL)
		c.Assert(err, jc.ErrorIsNil)

		err = s.JIMM.Database.GetApplicationOffer(ctx, &offer)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(offer.UserAccess(&testUser), gc.Equals, "read")

		conn3 := s.open(c, nil, "user3")
		defer conn3.Close()
		client3 := applicationoffers.NewClient(conn3)

		err = client3.RevokeOffer("test.user@external", "read", offerURL)
		c.Assert(err, gc.ErrorMatches, "unauthorized")
	*/
}

func (s *applicationOffersSuite) TestDestroyOffers(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	offerURL := "bob@external/model-1.test-offer1"

	// charlie will have read access
	// TODO (alesstimec) until i implement proper grant/revoke access
	// i need to fetch the offer so that i can manually set read
	// permission for charlie
	//
	//err = client.GrantOffer("charlie@external", "read", offerURL)
	//c.Assert(err, jc.ErrorIsNil)
	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = s.JIMM.Database.GetApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.Equals, nil)
	charlie := openfga.NewUser(&dbmodel.User{Username: "charlie@external"}, s.OFGAClient)
	err = charlie.SetApplicationOfferAccess(context.Background(), offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, gc.Equals, nil)

	// try to destroy offer that does not exist
	err = client.DestroyOffers(true, "bob@external/model-1.test-offer2")
	c.Assert(err, gc.ErrorMatches, "application offer not found")

	conn2 := s.open(c, nil, "charlie@external")
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	// charlie is not authorized to destroy the offer
	err = client2.DestroyOffers(true, offerURL)
	c.Assert(err, gc.ErrorMatches, "unauthorized")

	// bob can destroy the offer
	err = client.DestroyOffers(true, offerURL)
	c.Assert(err, jc.ErrorIsNil)

	offers, err := client.ListOffers(crossmodel.ApplicationOfferFilter{
		ModelName: s.Model.Name,
		OfferName: "test-offer1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationOffersSuite) TestFindApplicationOffers(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	// without filters
	_, err = client.FindApplicationOffers()
	c.Assert(err, gc.ErrorMatches, "at least one filter must be specified")

	offers, err := client.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		ModelName:       s.Model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)
	for i := range offers {
		// mask the charm URL as it changes depending on the test run order.
		offers[i].CharmURL = ""
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@external",
			Access:   "admin",
		}, {
			UserName: "bob@external",
			Access:   "admin",
		}, {
			UserName: auth.EveryoneUser,
			Access:   "read",
		}},
	}})

	// by default each offer is publicly readable -> charlie should be
	// able to find it
	conn2 := s.open(c, nil, "charlie@external")
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	offers, err = client2.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		OwnerName:       s.Model.OwnerUsername,
		ModelName:       s.Model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, gc.Equals, nil)
	for _, offer := range offers {
		// mask the charm URL as it changes depending on the test run order.
		offer.CharmURL = ""
	}
	c.Assert(offers, jc.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: auth.EveryoneUser,
			Access:   "read",
		}},
	}})
}

func (s *applicationOffersSuite) TestApplicationOffers(c *gc.C) {
	conn := s.open(c, nil, "bob@external")
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		s.Model.UUID.String,
		"test-app",
		[]string{s.endpoint.Name},
		"bob@external",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Error, gc.Equals, (*jujuparams.Error)(nil))

	url := "bob@external/model-1.test-offer1"
	offer, err := client.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)

	// mask the charm URL as it changes depending on the test run order.
	offer.CharmURL = ""
	sort.Slice(offer.Users, func(i, j int) bool {
		return offer.Users[i].UserName < offer.Users[j].UserName
	})
	c.Assert(offer, jc.DeepEquals, &crossmodel.ApplicationOfferDetails{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@external",
			Access:   "admin",
		}, {
			UserName: "bob@external",
			Access:   "admin",
		}, {
			UserName: auth.EveryoneUser,
			Access:   "read",
		}},
	})

	_, err = client.ApplicationOffer("charlie@external/model-1.test-offer2")
	c.Assert(err, gc.ErrorMatches, "application offer not found")

	conn2 := s.open(c, nil, "charlie@external")
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	offer, err = client2.ApplicationOffer(url)
	c.Assert(err, jc.ErrorIsNil)
	// mask the charm URL as it changes depending on the test run order.
	offer.CharmURL = ""
	c.Assert(offer, jc.DeepEquals, &crossmodel.ApplicationOfferDetails{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@external/model-1.test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "url",
			Role:      "provider",
			Interface: "http",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: auth.EveryoneUser,
			Access:   "read",
		}},
	})
}
