// Copyright 2020 Canonical Ltd.

package jimmdb_test

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/utils/v2"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/jem/jimmdb"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mgosession"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationOfferSuite struct {
	jemtest.IsolatedMgoSuite
	database *jimmdb.Database
}

var _ = gc.Suite(&applicationOfferSuite{})

func (s *applicationOfferSuite) SetUpTest(c *gc.C) {
	s.IsolatedMgoSuite.SetUpTest(c)
	pool := mgosession.NewPool(context.TODO(), s.Session, 1)
	s.database = jimmdb.NewDatabase(context.TODO(), pool, "jem")
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
	pool.Close()
	c.Assert(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *applicationOfferSuite) TearDownTest(c *gc.C) {
	s.database.Session.Close()
	s.database = nil
	s.IsolatedMgoSuite.TearDownTest(c)
}

func (s *applicationOfferSuite) checkDBOK(c *gc.C) {
	c.Check(s.database.Session.Ping(), gc.Equals, nil)
}

func (s *applicationOfferSuite) TestApplicationOffers(c *gc.C) {
	offer := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000001",
		OfferURL:               "user1@external/test-model:test-offer1",
		OwnerName:              "user1@external",
		ModelUUID:              "00000000-0000-0000-0000-000000000002",
		ModelName:              "test-model",
		OfferName:              "test-offer1",
		ApplicationName:        "test-application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
	}

	err := s.database.InsertApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.Equals, nil)

	err = s.database.InsertApplicationOffer(context.Background(), &offer)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrAlreadyExists)

	offer1 := mongodoc.ApplicationOffer{
		OfferUUID: offer.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer1)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer1, jemtest.CmpEquals(cmpopts.EquateEmpty()), offer)

	offer2 := mongodoc.ApplicationOffer{
		OfferUUID: "no-such-offer",
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer2)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	update := new(jimmdb.Update)
	update.Set("offer-name", "another-test-offer")
	update.Set("application-name", "another-test-application")
	update.Set("endpoints", []mongodoc.RemoteEndpoint{{Name: "ep4"}})
	err = s.database.UpdateApplicationOffer(context.Background(), &offer, update, true)
	c.Assert(err, gc.Equals, nil)

	offer3 := mongodoc.ApplicationOffer{
		OfferUUID: offer.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer3)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer3, jemtest.CmpEquals(cmpopts.EquateEmpty()), offer)

	err = s.database.UpdateApplicationOffer(context.Background(), &offer2, update, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	offer4 := mongodoc.ApplicationOffer{
		OfferURL: offer.OfferURL,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer4)
	c.Assert(err, gc.Equals, nil)
	c.Assert(offer4, jemtest.CmpEquals(cmpopts.EquateEmpty()), offer)

	offer5 := mongodoc.ApplicationOffer{
		OfferURL: "no such offer",
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer5)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveApplicationOffer(context.Background(), &offer)
	c.Assert(err, gc.Equals, nil)

	offer6 := mongodoc.ApplicationOffer{
		OfferUUID: offer.OfferUUID,
	}
	err = s.database.GetApplicationOffer(context.Background(), &offer6)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.database.RemoveApplicationOffer(context.Background(), &offer)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationOfferSuite) TestForEachApplicationOffer(c *gc.C) {
	ctx := context.Background()

	m1 := utils.MustNewUUID().String()
	m2 := utils.MustNewUUID().String()
	offer1 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000002",
		OwnerName:              "bob@external",
		ModelUUID:              m1,
		ModelName:              "test-model1",
		OfferName:              "test offer 1",
		ApplicationName:        "test application",
		ApplicationDescription: "test description",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: mongodoc.ApplicationOfferAccessMap{
			identchecker.Everyone: mongodoc.ApplicationOfferReadAccess,
			"alice":               mongodoc.ApplicationOfferConsumeAccess,
			"bob":                 mongodoc.ApplicationOfferAdminAccess,
		},
	}
	offer2 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000003",
		OwnerName:              "bob@external",
		ModelUUID:              m1,
		ModelName:              "test-model1",
		OfferName:              "test offer 2",
		ApplicationName:        "test application 1",
		ApplicationDescription: "test description 1",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: mongodoc.ApplicationOfferAccessMap{
			identchecker.Everyone: mongodoc.ApplicationOfferReadAccess,
			"alice":               mongodoc.ApplicationOfferConsumeAccess,
			"bob":                 mongodoc.ApplicationOfferAdminAccess,
		},
	}
	offer3 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000004",
		OwnerName:              "bob@external",
		ModelUUID:              m2,
		ModelName:              "test-model2",
		OfferName:              "test offer 1",
		ApplicationName:        "test application 2",
		ApplicationDescription: "test description 2",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: mongodoc.ApplicationOfferAccessMap{
			identchecker.Everyone: mongodoc.ApplicationOfferReadAccess,
			"alice":               mongodoc.ApplicationOfferConsumeAccess,
			"bob":                 mongodoc.ApplicationOfferAdminAccess,
		},
	}
	offer4 := mongodoc.ApplicationOffer{
		OfferUUID:              "00000000-0000-0000-0000-000000000005",
		OwnerName:              "bob@external",
		ModelUUID:              m2,
		ModelName:              "test-model2",
		OfferName:              "test offer 2",
		ApplicationName:        "test application 3",
		ApplicationDescription: "test description 3",
		Endpoints: []mongodoc.RemoteEndpoint{{
			Name: "ep1",
		}, {
			Name: "ep2",
		}, {
			Name: "ep3",
		}},
		Users: mongodoc.ApplicationOfferAccessMap{
			identchecker.Everyone: mongodoc.ApplicationOfferReadAccess,
			"alice":               mongodoc.ApplicationOfferConsumeAccess,
			"bob":                 mongodoc.ApplicationOfferAdminAccess,
		},
	}
	for _, offer := range []mongodoc.ApplicationOffer{offer1, offer2, offer3, offer4} {
		err := s.database.InsertApplicationOffer(ctx, &offer)
		c.Assert(err, gc.Equals, nil)
	}

	expect := []mongodoc.ApplicationOffer{offer1, offer2, offer3, offer4}
	err := s.database.ForEachApplicationOffer(ctx, nil, []string{"_id"}, func(o *mongodoc.ApplicationOffer) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected offer %q", o.OfferURL)
		}
		c.Check(o, jemtest.CmpEquals(cmpopts.EquateEmpty()), &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(expect, gc.HasLen, 0)

	expect = []mongodoc.ApplicationOffer{offer1, offer3}
	err = s.database.ForEachApplicationOffer(ctx, jimmdb.Eq("offer-name", "test offer 1"), []string{"_id"}, func(o *mongodoc.ApplicationOffer) error {
		if len(expect) < 1 {
			return errgo.Newf("unexpected offer %q", o.OfferURL)
		}
		c.Check(o, jemtest.CmpEquals(cmpopts.EquateEmpty()), &expect[0])
		expect = expect[1:]
		return nil
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(expect, gc.HasLen, 0)

	testError := errgo.New("test")
	err = s.database.ForEachApplicationOffer(ctx, nil, nil, func(o *mongodoc.ApplicationOffer) error {
		return testError
	})
	c.Check(errgo.Cause(err), gc.Equals, testError)
}
