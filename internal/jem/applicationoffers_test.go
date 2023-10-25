// Copyright 2020 Canonical Ltd.

package jem_test

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/identchecker"
	"github.com/juju/charm/v8"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"

	"github.com/canonical/jimm/internal/conv"
	"github.com/canonical/jimm/internal/jem/jimmdb"
	"github.com/canonical/jimm/internal/jemtest"
	"github.com/canonical/jimm/internal/mongodoc"
	"github.com/canonical/jimm/params"
)

type applicationoffersSuite struct {
	jemtest.BootstrapSuite

	endpoint state.Endpoint
}

var _ = gc.Suite(&applicationoffersSuite{})

func (s *applicationoffersSuite) SetUpTest(c *gc.C) {
	s.BootstrapSuite.SetUpTest(c)
	modelState, err := s.StatePool.Get(s.Model.UUID)
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

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetails(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offerURL := conv.ToOfferURL(s.Model.Path, "test-offer")

	d := jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			OfferURL: offerURL,
		},
	}
	err = s.JEM.GetApplicationOfferConsumeDetails(ctx, jemtest.Bob, "", &d, bakery.Version2)
	c.Assert(err, gc.Equals, nil)

	c.Check(d.Macaroon, gc.Not(gc.IsNil))
	d.Macaroon = nil
	c.Check(d.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	d.Offer.OfferUUID = ""
	c.Check(d, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferURL:               offerURL,
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Alias:         "controller-1",
			Addrs:         s.APIInfo(c).Addrs,
			CACert:        s.Controller.CACert,
		},
	})

	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, "charlie", offerURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, "alice", offerURL, jujuparams.OfferAdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.GetApplicationOfferConsumeDetails(ctx, jemtest.Alice, "charlie", &d, bakery.Version2)
	c.Assert(err, gc.Equals, nil)

	c.Check(d.Macaroon, gc.Not(gc.IsNil))
	d.Macaroon = nil
	c.Check(d.Offer.OfferUUID, gc.Matches, `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	d.Offer.OfferUUID = ""
	c.Check(d, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferURL:               offerURL,
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "charlie@external",
				Access:   "consume",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Alias:         "controller-1",
			Addrs:         s.APIInfo(c).Addrs,
			CACert:        s.Controller.CACert,
		},
	})
}

func (s *applicationoffersSuite) TestListApplicationOffers(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer1"))
	c.Assert(err, jc.ErrorIsNil)
	offer2, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer2"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, params.User("charlie"), offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.JEM.ListApplicationOffers(ctx, jemtest.NewIdentity("unknown-user"), jujuparams.OfferFilter{
		ModelName: s.Model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.JEM.ListApplicationOffers(ctx, jemtest.Charlie, jujuparams.OfferFilter{
		ModelName: s.Model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.JEM.ListApplicationOffers(ctx, jemtest.Bob, jujuparams.OfferFilter{
		ModelName: string(s.Model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   nil,
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: "charlie@external",
				Access:   "read",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     nil,
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer2.OfferUUID,
			OfferURL:               offer2.OfferURL,
			OfferName:              offer2.OfferName,
			ApplicationDescription: offer2.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   nil,
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer2.ApplicationName,
		CharmURL:        offer2.CharmURL,
		Connections:     nil,
	}})
}

func (s *applicationoffersSuite) TestModifyOfferAccess(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.Model.Path, "test-offer1"),
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.DB.UpdateApplicationOffer(ctx, &offer1, new(jimmdb.Update).Set("users.charlie", mongodoc.ApplicationOfferNoAccess), true)
	c.Assert(err, jc.ErrorIsNil)
	err = s.JEM.DB.UpdateApplicationOffer(ctx, &offer1, new(jimmdb.Update).Set("users."+identchecker.Everyone, mongodoc.ApplicationOfferNoAccess), true)
	c.Assert(err, jc.ErrorIsNil)

	// charlie does not have permission
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Charlie, "test-user", offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	err = s.JEM.DB.UpdateApplicationOffer(ctx, &offer1, new(jimmdb.Update).Set("users.charlie", mongodoc.ApplicationOfferConsumeAccess), true)
	c.Assert(err, jc.ErrorIsNil)

	// user2 has consume permission
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Charlie, "test-user", offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	// try granting unknown access level
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, "test-user", offer1.OfferURL, jujuparams.OfferAccessPermission("unknown"))
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrBadRequest)

	// try granting permission on an offer that does not exist
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, "test-user", "no such offer", jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// bob is an admin - this should pass
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, "test-user", offer1.OfferURL, jujuparams.OfferAdminAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)
	access := offer1.Users["test-user"]
	c.Assert(access, gc.Equals, mongodoc.ApplicationOfferAdminAccess)

	// bob is an admin - this should pass and access level be set to "read"
	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Bob, "test-user", offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)
	access = offer1.Users["test-user"]
	c.Assert(access, gc.Equals, mongodoc.ApplicationOfferReadAccess)

	// user2 is has consume access - unauthorized
	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Charlie, "test-user", offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	err = s.JEM.DB.UpdateApplicationOffer(ctx, &offer1, new(jimmdb.Update).Set("users.charlie", mongodoc.ApplicationOfferNoAccess), true)
	c.Assert(err, jc.ErrorIsNil)

	// user2 is does not have access - not found
	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Charlie, "test-user", offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// try revoking unknown access level
	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Bob, "test-user", offer1.OfferURL, jujuparams.OfferAccessPermission("unknown"))
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrBadRequest)

	// try revoking for an offer that does not exist
	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Bob, "test-user", "no such offer", jujuparams.OfferReadAccess)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestDestroyOffer(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.Model.Path, "test-offer1"),
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, gc.Equals, nil)
	err = s.JEM.DB.UpdateApplicationOffer(ctx, &offer1, new(jimmdb.Update).Set("users."+identchecker.Everyone, mongodoc.ApplicationOfferNoAccess), true)
	c.Assert(err, gc.Equals, nil)

	// bob is an admin - this should pass
	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, params.User("charlie"), offer1.OfferURL, jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	// charlie has consumer access - unauthorized
	err = s.JEM.DestroyOffer(ctx, jemtest.Charlie, offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrUnauthorized)

	// user3 has no access - not found
	err = s.JEM.DestroyOffer(ctx, jemtest.NewIdentity("user3"), offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// bob is admin
	err = s.JEM.DestroyOffer(ctx, jemtest.Bob, offer1.OfferURL, true)
	c.Assert(err, gc.Equals, nil)

	offer2 := offer1
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// offer not found
	err = s.JEM.DestroyOffer(ctx, jemtest.Bob, offer1.OfferURL, true)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestFindApplicationOffers(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer1"))
	c.Assert(err, jc.ErrorIsNil)
	offer2, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer2"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.GrantOfferAccess(ctx, jemtest.Bob, params.User("charlie"), offer1.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Bob, params.User(identchecker.Everyone), offer2.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.JEM.FindApplicationOffers(ctx, jemtest.NewIdentity("unknown-user"), jujuparams.OfferFilter{
		ModelName: s.Model.UUID,
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, gc.HasLen, 0)

	results, err = s.JEM.FindApplicationOffers(ctx, jemtest.Charlie, jujuparams.OfferFilter{
		ModelName: string(s.Model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, jc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "charlie@external",
				Access:   "read",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}})

	results, err = s.JEM.FindApplicationOffers(ctx, jemtest.Bob, jujuparams.OfferFilter{
		ModelName: string(s.Model.Path.Name),
	})
	c.Assert(err, gc.Equals, nil)
	c.Assert(results, jc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: "charlie@external",
				Access:   "read",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}, {
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer2.OfferUUID,
			OfferURL:               offer2.OfferURL,
			OfferName:              offer2.OfferName,
			ApplicationDescription: offer2.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}},
		},
		ApplicationName: offer2.ApplicationName,
		CharmURL:        offer2.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	}})
}

func (s *applicationoffersSuite) TestGetApplicationOffer(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.Model.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer1"))
	c.Assert(err, jc.ErrorIsNil)
	offer2, err := s.JEM.GetApplicationOffer(ctx, jemtest.Bob, conv.ToOfferURL(s.Model.Path, "test-offer2"))
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.RevokeOfferAccess(ctx, jemtest.Bob, params.User(identchecker.Everyone), offer2.OfferURL, jujuparams.OfferReadAccess)
	c.Assert(err, jc.ErrorIsNil)

	// "unknown-user" does not have access to offer2
	_, err = s.JEM.GetApplicationOffer(ctx, jemtest.NewIdentity("unknown-user"), offer2.OfferURL)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)

	// "unknown-user" has read access to offer1
	offerDetails, err := s.JEM.GetApplicationOffer(ctx, jemtest.NewIdentity("unknown-user"), offer1.OfferURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerDetails, jc.DeepEquals, &jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
		Connections:     []jujuparams.OfferConnection{},
	})

	// "bob" is admin and will see addition details of offer1
	offerDetails, err = s.JEM.GetApplicationOffer(ctx, jemtest.Bob, offer1.OfferURL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offerDetails, jc.DeepEquals, &jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.Model.UUID).String(),
			OfferUUID:              offer1.OfferUUID,
			OfferURL:               offer1.OfferURL,
			OfferName:              offer1.OfferName,
			ApplicationDescription: offer1.ApplicationDescription,
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      charm.RoleProvider,
				Interface: "http",
				Limit:     0,
			}},
			Spaces:   []jujuparams.RemoteSpace{},
			Bindings: nil,
			Users: []jujuparams.OfferUserDetails{{
				UserName: "bob@external",
				Access:   "admin",
			}, {
				UserName: "everyone@external",
				Access:   "read",
			}},
		},
		ApplicationName: offer1.ApplicationName,
		CharmURL:        offer1.CharmURL,
	})

	// bob is admin but still cannot get application offers that do not exist
	_, err = s.JEM.GetApplicationOffer(ctx, jemtest.Bob, "no-such-offer")
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}

func (s *applicationoffersSuite) TestUpdateApplicationOffer(c *gc.C) {
	ctx := context.Background()

	err := s.JEM.Offer(ctx, jemtest.Bob, jujuparams.AddApplicationOffer{
		ModelTag:               names.NewModelTag(s.Model.UUID).String(),
		OfferName:              "test-offer1",
		ApplicationName:        "test-app",
		ApplicationDescription: "test application description",
		Endpoints: map[string]string{
			s.endpoint.Relation.Name: s.endpoint.Relation.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offer1 := mongodoc.ApplicationOffer{
		OfferURL: conv.ToOfferURL(s.Model.Path, "test-offer1"),
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer1)
	c.Assert(err, jc.ErrorIsNil)

	modelState, err := s.StatePool.Get(s.Model.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()

	appOfferState := state.NewApplicationOffers(modelState.State)
	_, err = appOfferState.UpdateOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:              offer1.OfferName,
		Owner:                  offer1.OwnerName,
		ApplicationName:        offer1.ApplicationName,
		ApplicationDescription: "changed test application description",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.JEM.UpdateApplicationOffer(ctx, s.Controller.Path, offer1.OfferUUID, false)
	c.Assert(err, jc.ErrorIsNil)

	offer2 := mongodoc.ApplicationOffer{
		OfferUUID: offer1.OfferUUID,
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(offer2.ApplicationDescription, gc.Equals, "changed test application description")

	err = s.JEM.UpdateApplicationOffer(ctx, s.Controller.Path, offer1.OfferUUID, true)
	c.Assert(err, jc.ErrorIsNil)

	offer3 := mongodoc.ApplicationOffer{
		OfferUUID: offer1.OfferUUID,
	}
	err = s.JEM.DB.GetApplicationOffer(ctx, &offer3)
	c.Assert(errgo.Cause(err), gc.Equals, params.ErrNotFound)
}
