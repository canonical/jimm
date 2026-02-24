// Copyright 2025 Canonical.

package testing

import (
	"context"
	"sort"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/juju/charm/v12"
	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

func SetupAppOfferTest(c *qt.C) (jimmtest.JimmWithControllers, *dbmodel.Model) {
	s := jimmtest.SetupJimmWithControllers(c)
	model := s.CreateModelForBob(c)

	// App will be cleaned up when the model is destroyed.
	s.DeployApplication(c, s.AdminUser, model.Tag(), jimmtest.DeployApplicationParams{
		App:   "test-app",
		Charm: "juju-qa-dummy-sink",
	})

	return s, model
}

func TestOffer(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(model.UUID.String, "no-such-app", []string{"source"}, "bob@canonical.com", "test-offer-foo", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Not(qt.IsNil))
	c.Assert(results[0].Error.Code, qt.Equals, "not found")

	conn1 := s.Open(c, nil, "charlie@canonical.com", nil)
	defer conn1.Close()
	client1 := applicationoffers.NewClient(conn1)

	results, err = client1.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer-2", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error.Code, qt.Equals, "unauthorized access")
}

func TestCreateMultipleOffersForSameApp(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	// Creating an offer with the same name as above.
	results, err = client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.ErrorMatches, `offer bob@canonical.com/`+model.Name+`.test-offer already exists, please use a different name.*`)

	// Creating an offer with a new name.
	results, err = client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer-foo", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))
}

func TestGetConsumeDetails(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	ourl := &crossmodel.OfferURL{
		User:            "bob@canonical.com",
		ModelName:       model.Name,
		ApplicationName: "test-offer",
	}
	details, err := client.GetConsumeDetails(ourl.Path())
	c.Assert(err, qt.Equals, nil)
	c.Check(details.Macaroon, qt.Not(qt.IsNil))
	details.Macaroon = nil
	c.Check(details.Offer.OfferUUID, qt.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	details.Offer.OfferUUID = ""
	info := s.GetControllerConfig(c, model.Controller.Name)

	sort.Slice(details.Offer.Users, func(i, j int) bool {
		return details.Offer.Users[i].UserName < details.Offer.Users[j].UserName
	})
	c.Check(details, qt.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         model.Tag().String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "source",
				Role:      "provider",
				Interface: "dummy-token",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "bob@canonical.com",
				Access:   "admin",
			}, {
				UserName: ofganames.EveryoneUser,
				Access:   "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: model.Controller.Tag().String(),
			Addrs:         info.Addrs,
			Alias:         model.Controller.Name,
			CACert:        info.CACert,
		},
	})

	ourl2 := &crossmodel.OfferURL{
		ModelName:       model.Name,
		ApplicationName: "test-offer",
	}

	details, err = client.GetConsumeDetails(ourl2.Path())
	c.Assert(err, qt.Equals, nil)
	c.Check(details.Macaroon, qt.Not(qt.IsNil))
	details.Macaroon = nil
	c.Check(details.Offer.OfferUUID, qt.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	details.Offer.OfferUUID = ""
	sort.Slice(details.Offer.Users, func(j, k int) bool {
		return details.Offer.Users[j].UserName < details.Offer.Users[k].UserName
	})
	c.Check(details, qt.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         model.Tag().String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "source",
				Role:      "provider",
				Interface: "dummy-token",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "alice@canonical.com",
				Access:   "admin",
			}, {
				UserName: "bob@canonical.com",
				Access:   "admin",
			}, {
				UserName: ofganames.EveryoneUser,
				Access:   "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: model.Controller.Tag().String(),
			Addrs:         info.Addrs,
			Alias:         model.Controller.Name,
			CACert:        info.CACert,
		},
	})
}

func TestGetConsumeDetailsWithConsumeAccess(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	ourl := &crossmodel.OfferURL{
		User:            "bob@canonical.com",
		ModelName:       model.Name,
		ApplicationName: "test-offer",
	}

	user := "regular.joe@canonical.com"
	err = client.GrantOffer(user, string(jujuparams.OfferConsumeAccess), ourl.String())
	c.Assert(err, qt.Equals, nil)

	info := s.GetControllerConfig(c, model.Controller.Name)

	conn1 := s.Open(c, nil, user, nil)
	defer conn.Close()
	client1 := applicationoffers.NewClient(conn1)

	details, err := client1.GetConsumeDetails(ourl.String())
	c.Assert(err, qt.Equals, nil)
	c.Check(details.Macaroon, qt.Not(qt.IsNil))
	details.Macaroon = nil
	c.Check(details.Offer.OfferUUID, qt.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	details.Offer.OfferUUID = ""
	sort.Slice(details.Offer.Users, func(j, k int) bool {
		return details.Offer.Users[j].UserName < details.Offer.Users[k].UserName
	})
	c.Check(details, qt.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         model.Tag().String(),
			OfferURL:               ourl.Path(),
			OfferName:              "test-offer",
			ApplicationDescription: "test offer description",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "source",
				Role:      "provider",
				Interface: "dummy-token",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: ofganames.EveryoneUser,
				Access:   "read",
			}, {
				UserName: user,
				Access:   "consume",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: model.Controller.Tag().String(),
			Addrs:         info.Addrs,
			Alias:         model.Controller.Name,
			CACert:        info.CACert,
		},
	})

	err = client.RevokeOffer(user, string(jujuparams.OfferConsumeAccess), ourl.String())
	c.Assert(err, qt.Equals, nil)

	_, err = client1.GetConsumeDetails(ourl.String())
	c.Assert(err, qt.ErrorMatches, "unauthorized")
}

func TestListApplicationOffers(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	// without filters
	_, err = client.ListOffers()
	c.Assert(err, qt.ErrorMatches, `at least one filter must be specified \(bad request\)`)

	offers, err := client.ListOffers(crossmodel.ApplicationOfferFilter{
		OwnerName:       model.Owner.Name,
		ModelName:       model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, qt.Equals, nil)

	for i, offer := range offers {
		// mask the charm URL as it changes depending on the test
		// run order.
		offer.CharmURL = ""
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Assert(offers, qt.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@canonical.com/" + model.Name + ".test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      "provider",
			Interface: "dummy-token",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@canonical.com",
			Access:   "admin",
		}, {
			UserName: "bob@canonical.com",
			Access:   "admin",
		}, {
			UserName: ofganames.EveryoneUser,
			Access:   "read",
		}},
	}})
}

func TestModifyOfferAccess(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	ctx := context.Background()

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	offerURL := "bob@canonical.com/" + model.Name + ".test-offer1"

	err = client.RevokeOffer(ofganames.EveryoneUser, "read", offerURL)
	c.Assert(err, qt.IsNil)

	err = client.GrantOffer("test.user@canonical.com", "unknown", offerURL)
	c.Assert(err, qt.ErrorMatches, `"unknown" offer access not valid`)

	err = client.GrantOffer("test.user@canonical.com", "admin", offerURL)
	c.Assert(err, qt.IsNil)

	err = client.GrantOffer("test.user@canonical.com", "admin", offerURL)
	c.Assert(err, qt.IsNil)

	testUser := openfga.NewUser(
		&dbmodel.Identity{
			Name: "test.user@canonical.com",
		},
		s.OFGAClient,
	)

	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = s.JIMM.Database.GetApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	testUserAccess := testUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
	c.Assert(testUserAccess, qt.Equals, ofganames.AdministratorRelation)

	err = client.RevokeOffer("test.user@canonical.com", "admin", offerURL)
	c.Assert(err, qt.IsNil)

	testUserAccess = testUser.GetApplicationOfferAccess(ctx, offer.ResourceTag())
	c.Assert(testUserAccess, qt.Equals, ofganames.NoRelation)

	conn3 := s.Open(c, nil, "user3", nil)
	defer conn3.Close()
	client3 := applicationoffers.NewClient(conn3)

	err = client3.RevokeOffer("test.user@canonical.com", "read", offerURL)
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	err = client.GrantOffer("test.user@canonical.com", "admin", offerURL)
	c.Assert(err, qt.IsNil)

	err = client.GrantOffer("test.user@canonical.com", "admin", offerURL)
	c.Assert(err, qt.IsNil)
}

func TestDestroyOffers(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	offerURL := "bob@canonical.com/" + model.Name + ".test-offer1"

	// charlie will have read access
	offer := dbmodel.ApplicationOffer{
		URL: offerURL,
	}
	err = s.JIMM.Database.GetApplicationOffer(context.Background(), &offer)
	c.Assert(err, qt.Equals, nil)

	charlieIdentity, err := dbmodel.NewIdentity("charlie@canonical.com")
	c.Assert(err, qt.IsNil)
	charlie := openfga.NewUser(charlieIdentity, s.OFGAClient)
	err = charlie.SetApplicationOfferAccess(context.Background(), offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.Equals, nil)

	// try to destroy offer that does not exist
	err = client.DestroyOffers(true, "bob@canonical.com/model-1.test-offer2")
	c.Assert(err, qt.ErrorMatches, "application offer not found")

	conn2 := s.Open(c, nil, "charlie@canonical.com", nil)
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	// charlie is not authorized to destroy the offer
	err = client2.DestroyOffers(true, offerURL)
	c.Assert(err, qt.ErrorMatches, "unauthorized")

	// bob can destroy the offer
	err = client.DestroyOffers(true, offerURL)
	c.Assert(err, qt.IsNil)

	offers, err := client.ListOffers(crossmodel.ApplicationOfferFilter{
		OwnerName: model.Owner.Name,
		ModelName: model.Name,
		OfferName: "test-offer1",
	})
	c.Assert(err, qt.IsNil)
	c.Assert(offers, qt.HasLen, 0)
}

func TestFindApplicationOffers(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	results, err = client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer2",
		"test offer 2 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	// without filters
	_, err = client.FindApplicationOffers()
	c.Assert(err, qt.ErrorMatches, "at least one filter must be specified")

	offers, err := client.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		OwnerName:       model.OwnerIdentityName,
		ModelName:       model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, qt.Equals, nil)
	for i := range offers {
		// mask the charm URL as it changes depending on the test run order.
		offers[i].CharmURL = ""
		sort.Slice(offers[i].Users, func(j, k int) bool {
			return offers[i].Users[j].UserName < offers[i].Users[k].UserName
		})
	}
	c.Assert(offers, qt.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@canonical.com/" + model.Name + ".test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      "provider",
			Interface: "dummy-token",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@canonical.com",
			Access:   "admin",
		}, {
			UserName: "bob@canonical.com",
			Access:   "admin",
		}, {
			UserName: ofganames.EveryoneUser,
			Access:   "read",
		}},
	}})

	// by default each offer is publicly readable -> charlie should be
	// able to find it
	conn2 := s.Open(c, nil, "charlie@canonical.com", nil)
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	offers, err = client2.FindApplicationOffers(crossmodel.ApplicationOfferFilter{
		OwnerName:       model.OwnerIdentityName,
		ModelName:       model.Name,
		ApplicationName: "test-app",
		OfferName:       "test-offer1",
	})
	c.Assert(err, qt.Equals, nil)
	for _, offer := range offers {
		// mask the charm URL as it changes depending on the test run order.
		offer.CharmURL = ""
	}
	c.Assert(offers, qt.DeepEquals, []*crossmodel.ApplicationOfferDetails{{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@canonical.com/" + model.Name + ".test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      "provider",
			Interface: "dummy-token",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: ofganames.EveryoneUser,
			Access:   "read",
		}},
	}})
}

func TestApplicationOffers(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(
		model.UUID.String,
		"test-app",
		[]string{"source"},
		"bob@canonical.com",
		"test-offer1",
		"test offer 1 description",
	)
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	url := "bob@canonical.com/" + model.Name + ".test-offer1"
	offer, err := client.ApplicationOffer(url)
	c.Assert(err, qt.IsNil)

	// mask the charm URL as it changes depending on the test run order.
	offer.CharmURL = ""
	sort.Slice(offer.Users, func(i, j int) bool {
		return offer.Users[i].UserName < offer.Users[j].UserName
	})
	c.Assert(offer, qt.DeepEquals, &crossmodel.ApplicationOfferDetails{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@canonical.com/" + model.Name + ".test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      "provider",
			Interface: "dummy-token",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: "alice@canonical.com",
			Access:   "admin",
		}, {
			UserName: "bob@canonical.com",
			Access:   "admin",
		}, {
			UserName: ofganames.EveryoneUser,
			Access:   "read",
		}},
	})

	_, err = client.ApplicationOffer("charlie@canonical.com/" + model.Name + ".test-offer2")
	c.Assert(err, qt.ErrorMatches, "application offer not found")

	conn2 := s.Open(c, nil, "charlie@canonical.com", nil)
	defer conn2.Close()
	client2 := applicationoffers.NewClient(conn2)

	offer, err = client2.ApplicationOffer(url)
	c.Assert(err, qt.IsNil)
	// mask the charm URL as it changes depending on the test run order.
	offer.CharmURL = ""
	c.Assert(offer, qt.DeepEquals, &crossmodel.ApplicationOfferDetails{
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test offer 1 description",
		OfferURL:               "bob@canonical.com/" + model.Name + ".test-offer1",
		Endpoints: []charm.Relation{{
			Name:      "source",
			Role:      "provider",
			Interface: "dummy-token",
		}},
		Users: []crossmodel.OfferUserDetails{{
			UserName: ofganames.EveryoneUser,
			Access:   "read",
		}},
	})
}
