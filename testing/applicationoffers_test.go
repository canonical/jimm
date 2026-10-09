// Copyright 2025 Canonical.

package testing

import (
	"context"
	"fmt"
	"sort"
	"testing"

	qt "github.com/frankban/quicktest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/charm/v12"
	"github.com/juju/juju/api/client/applicationoffers"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/canonical/jimm/v3/internal/dbmodel"
	"github.com/canonical/jimm/v3/internal/openfga"
	ofganames "github.com/canonical/jimm/v3/internal/openfga/names"
	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
	jimmnames "github.com/canonical/jimm/v3/pkg/names"
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

	conn3 := s.Open(c, nil, "user3@canonical.com", nil)
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

func TestApplicationOfferDanglingOnRead(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)
	ctx := context.Background()

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	url := "bob@canonical.com/" + model.Name + ".test-offer1"

	// Simulate dangling offer
	offer := dbmodel.ApplicationOffer{
		ID:      1,
		UUID:    "00000001-0000-0000-0000-000000000001",
		Name:    "test-offer1",
		ModelID: model.ID,
		URL:     url,
	}
	err := s.JIMM.Database.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	err = s.JIMM.OpenFGAClient.AddModelApplicationOffer(ctx, model.ResourceTag(), offer.ResourceTag())
	c.Assert(err, qt.IsNil)

	everyoneIdentity := &dbmodel.Identity{Name: ofganames.EveryoneUser}
	everyoneUser := openfga.NewUser(everyoneIdentity, s.JIMM.OpenFGAClient)
	err = everyoneUser.SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// Trigger sync cleanup
	_, err = client.ApplicationOffer(url)
	c.Assert(err, qt.ErrorMatches, "application offer not found")

	// Ensure offer has been removed from the database
	offer = dbmodel.ApplicationOffer{
		URL: url,
	}
	err = s.JIMM.Database.GetApplicationOffer(ctx, &offer)
	c.Assert(err, qt.ErrorMatches, "application offer not found")

	// Ensure offer doesn't show up in endpoint either
	_, err = client.ApplicationOffer(url)
	c.Assert(err, qt.ErrorMatches, "application offer not found")
}

func TestApplicationOfferDanglingOnCreate(t *testing.T) {
	c := qt.New(t)
	s, model := SetupAppOfferTest(c)
	ctx := context.Background()

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	url := "bob@canonical.com/" + model.Name + ".test-offer1"

	// Simulate a dangling offer
	offer := dbmodel.ApplicationOffer{
		ID:      1,
		UUID:    "00000001-0000-0000-0000-000000000001",
		Name:    "test-offer1",
		ModelID: model.ID,
		URL:     url,
	}
	err := s.JIMM.Database.AddApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	err = s.JIMM.OpenFGAClient.AddModelApplicationOffer(ctx, model.ResourceTag(), offer.ResourceTag())
	c.Assert(err, qt.IsNil)

	everyoneIdentity := &dbmodel.Identity{Name: ofganames.EveryoneUser}
	everyoneUser := openfga.NewUser(everyoneIdentity, s.JIMM.OpenFGAClient)
	err = everyoneUser.SetApplicationOfferAccess(ctx, offer.ResourceTag(), ofganames.ReaderRelation)
	c.Assert(err, qt.IsNil)

	// Creating an offer with the same URL should succeed: JIMM detects
	// the dangling record, cleans it up, and creates the real offer.
	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer1", "test offer description")
	c.Assert(err, qt.IsNil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	// The offer should now be fetchable and reflect the real controller state.
	details, err := client.ApplicationOffer(url)
	c.Assert(err, qt.IsNil)
	c.Assert(details.OfferName, qt.Equals, "test-offer1")
}

// TestDischargeOfferConsumeViaIdPGroup verifies JIMM's macaroon discharger
// grants consume access to an offer via IdP group membership resolved from
// the hook-service, as requested by a Juju controller when a cross-model
// relation is established.
func TestDischargeOfferConsumeViaIdPGroup(t *testing.T) {
	c := qt.New(t)
	ctx := context.Background()

	// Step 1. Create app and offers it out via bob.
	s := jimmtest.SetupJimmWithControllers(c, jimmtest.WithHookServiceGroupFetcher())
	model := s.CreateModelForBob(c)

	s.DeployApplication(c, s.AdminUser, model.Tag(), jimmtest.DeployApplicationParams{
		App:   "test-app",
		Charm: "juju-qa-dummy-sink",
	})

	conn := s.Open(c, nil, "bob@canonical.com", nil)
	defer conn.Close()
	client := applicationoffers.NewClient(conn)

	results, err := client.Offer(model.UUID.String, "test-app", []string{"source"}, "bob@canonical.com", "test-offer", "test offer description")
	c.Assert(err, qt.Equals, nil)
	c.Assert(results, qt.HasLen, 1)
	c.Assert(results[0].Error, qt.Equals, (*jujuparams.Error)(nil))

	offer := dbmodel.ApplicationOffer{URL: "bob@canonical.com/" + model.Name + ".test-offer"}
	err = s.JIMM.Database.GetApplicationOffer(ctx, &offer)
	c.Assert(err, qt.IsNil)

	// Step 2.  Grant consume access on the offer to the "canonical" IdP
	// group (seeded our by local/hook-service/entrypoint.sh).
	// Its members hold no direc tuples on the offer.
	err = s.OFGAClient.AddRelation(ctx, openfga.Tuple{
		Object:   ofganames.ConvertTagWithRelation(jimmnames.NewIdPGroupTag("canonical"), ofganames.MemberRelation),
		Relation: ofganames.ConsumerRelation,
		Target:   ofganames.ConvertTag(offer.ResourceTag()),
	})
	c.Assert(err, qt.IsNil)

	// Step 3. Setup discharger and hit discharge endpoint
	// with the correct condition and tuples for auth.
	locator := httpbakery.NewThirdPartyLocator(nil, nil)
	locator.AllowInsecure()
	oven := bakery.NewOven(bakery.OvenParams{
		Key:      bakery.MustGenerateKey(),
		Locator:  locator,
		Location: "test-controller",
	})
	discharge := func(user string) (macaroon.Slice, error) {
		m, err := oven.NewMacaroon(ctx, bakery.LatestVersion, []checkers.Caveat{{
			Location:  s.Server.URL + "/macaroons",
			Condition: fmt.Sprintf("is-consumer %s %s", names.NewUserTag(user), offer.UUID),
		}}, bakery.NoOp)
		c.Assert(err, qt.IsNil)
		return httpbakery.NewClient().DischargeAll(ctx, m)
	}

	// Step 4. Test 2 scenarios, happy and unhappy paths.
	ms, err := discharge("jimm-group-user@canonical.com")
	c.Assert(err, qt.IsNil)
	c.Assert(checkers.InferDeclared(nil, ms), qt.DeepEquals, map[string]string{"offer-uuid": offer.UUID})

	_, err = discharge("nobody@canonical.com")
	c.Assert(err, qt.ErrorMatches, ".*cannot discharge: permission denied")
}
