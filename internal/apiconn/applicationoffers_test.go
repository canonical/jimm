// Copyright 2020 Canonical Ltd.

package apiconn_test

import (
	"context"
	"sort"

	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	gc "gopkg.in/check.v1"
	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon-bakery.v2/bakery"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/internal/mongodoc"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationoffersSuite struct {
	jemtest.JujuConnSuite

	cache *apiconn.Cache
	conn  *apiconn.Conn
	model mongodoc.Model
}

var _ = gc.Suite(&applicationoffersSuite{})

func (s *applicationoffersSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.cache = apiconn.NewCache(apiconn.CacheParams{})

	ctx := context.Background()
	var err error
	s.conn, err = s.cache.OpenAPI(ctx, s.ControllerConfig.ControllerUUID(), func() (api.Connection, *api.Info, error) {
		apiInfo := s.APIInfo(c)
		return apiOpen(
			&api.Info{
				Addrs:    apiInfo.Addrs,
				CACert:   apiInfo.CACert,
				Tag:      apiInfo.Tag,
				Password: apiInfo.Password,
			},
			api.DialOpts{},
		)
	})
	c.Assert(err, gc.Equals, nil)

	s.model.Path = params.EntityPath{
		User: "test-user",
		Name: "test-model",
	}

	err = s.conn.CreateModel(ctx, &s.model)
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationoffersSuite) TearDownTest(c *gc.C) {
	if s.conn != nil {
		s.conn.Close()
	}
	if s.cache != nil {
		s.cache.Close()
	}
	s.JujuConnSuite.TearDownTest(c)
}

func (s *applicationoffersSuite) TestOffer(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.NotNil)
	apiErr, ok := errgo.Cause(err).(*apiconn.APIError)
	c.Assert(ok, gc.Equals, true)
	c.Assert(apiErr.ParamsError().Message, gc.Matches, ".* application offer already exists")
}

func (s *applicationoffersSuite) TestOfferError(c *gc.C) {
	err := s.conn.Offer(context.Background(), jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			"url": "url",
		},
	})
	c.Check(err, gc.ErrorMatches, `api error: getting offered application test-app: application "test-app" not found`)
}

func (s *applicationoffersSuite) TestListApplicationOffersError(c *gc.C) {
	_, err := s.conn.ListApplicationOffers(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, `api error: at least one offer filter is required`)
}

func (s *applicationoffersSuite) TestListApplicationOffersNoOffers(c *gc.C) {
	offers, err := s.conn.ListApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestListApplicationOffersMatching(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = "test-user@external/test-model.test-offer"
	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{info})
}

func (s *applicationoffersSuite) TestListApplicationOffersNoMatch(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName:       string(s.model.Path.User) + "@external",
		ModelName:       string(s.model.Path.Name),
		ApplicationName: "no-such-app",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestFindApplicationOffersError(c *gc.C) {
	_, err := s.conn.FindApplicationOffers(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, `api error: at least one offer filter is required`)
}

func (s *applicationoffersSuite) TestFindApplicationOffersNoOffers(c *gc.C) {
	offers, err := s.conn.FindApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestFindApplicationOffersMatching(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = "test-user@external/test-model.test-offer"
	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	offers, err := s.conn.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{info})
}

func (s *applicationoffersSuite) TestFindApplicationOffersNoMatch(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offers, err := s.conn.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName:       string(s.model.Path.User) + "@external",
		ModelName:       string(s.model.Path.Name),
		ApplicationName: "no-such-app",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestGetApplicationOffer(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = "test-user@external/test-model.test-offer"
	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer",
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
				Limit:     0,
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "admin",
				DisplayName: "admin",
				Access:      string(jujuparams.OfferAdminAccess),
			}, {
				UserName: "everyone@external",
				Access:   string(jujuparams.OfferReadAccess),
			}},
		},
		ApplicationName: "test-app",
	})
}

func (s *applicationoffersSuite) TestGetApplicationOfferNotFound(c *gc.C) {
	ctx := context.Background()

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = "test-user@external/test-model.test-offer"
	err := s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.ErrorMatches, `api error: application offer "test-user@external/test-model.test-offer" not found`)
}

func (s *applicationoffersSuite) TestGrantApplicationOfferAccess(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offerURL := "test-user@external/test-model.test-offer"

	err = s.conn.GrantApplicationOfferAccess(ctx, offerURL, params.User("test-user-2"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = offerURL
	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer",
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
				Limit:     0,
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "admin",
				DisplayName: "admin",
				Access:      string(jujuparams.OfferAdminAccess),
			}, {
				UserName: "everyone@external",
				Access:   string(jujuparams.OfferReadAccess),
			}, {
				UserName: "test-user-2@external",
				Access:   string(jujuparams.OfferConsumeAccess),
			}},
		},
		ApplicationName: "test-app",
	})
}

func (s *applicationoffersSuite) TestGrantApplicationOfferAccessNotFound(c *gc.C) {
	ctx := context.Background()
	offerURL := "test-user@external/test-model.test-offer"

	err := s.conn.GrantApplicationOfferAccess(ctx, offerURL, params.User("test-user-2"), jujuparams.OfferConsumeAccess)
	c.Check(err, gc.ErrorMatches, `api error: application offer "test-offer" not found`)
}

func (s *applicationoffersSuite) TestRevokeApplicationOfferAccess(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offerURL := "test-user@external/test-model.test-offer"

	err = s.conn.GrantApplicationOfferAccess(ctx, offerURL, params.User("test-user-2"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetails
	info.OfferURL = offerURL
	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer",
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
				Limit:     0,
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "admin",
				DisplayName: "admin",
				Access:      string(jujuparams.OfferAdminAccess),
			}, {
				UserName: "everyone@external",
				Access:   string(jujuparams.OfferReadAccess),
			}, {
				UserName: "test-user-2@external",
				Access:   string(jujuparams.OfferConsumeAccess),
			}},
		},
		ApplicationName: "test-app",
	})

	err = s.conn.RevokeApplicationOfferAccess(ctx, offerURL, params.User("test-user-2"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.conn.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer",
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
				Limit:     0,
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName:    "admin",
				DisplayName: "admin",
				Access:      string(jujuparams.OfferAdminAccess),
			}, {
				UserName: "everyone@external",
				Access:   string(jujuparams.OfferReadAccess),
			}, {
				UserName: "test-user-2@external",
				Access:   string(jujuparams.OfferReadAccess),
			}},
		},
		ApplicationName: "test-app",
	})
}

func (s *applicationoffersSuite) TestRevokeApplicationOfferAccessNotFound(c *gc.C) {
	ctx := context.Background()
	offerURL := "test-user@external/test-model.test-offer"

	err := s.conn.RevokeApplicationOfferAccess(ctx, offerURL, params.User("test-user-2"), jujuparams.OfferConsumeAccess)
	c.Check(err, gc.ErrorMatches, `api error: application offer "test-offer" not found`)
}

func (s *applicationoffersSuite) TestDestroyApplicationOffer(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 1)

	offerURL := "test-user@external/test-model.test-offer"
	err = s.conn.DestroyApplicationOffer(ctx, offerURL, false)
	c.Assert(err, gc.Equals, nil)

	offers, err = s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: string(s.model.Path.User) + "@external",
		ModelName: string(s.model.Path.Name),
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetails(c *gc.C) {
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
	ep, err := app.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.model.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ConsumeOfferDetails
	info.Offer = &jujuparams.ApplicationOfferDetails{
		OfferURL: "test-user@external/test-model.test-offer",
	}
	err = s.conn.GetApplicationOfferConsumeDetails(ctx, params.User("test-user"), &info, bakery.Version2)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.Offer.OfferUUID, gc.Not(gc.Equals), "")
	info.Offer.OfferUUID = ""
	c.Check(info.Macaroon, gc.Not(gc.IsNil))
	info.Macaroon = nil
	c.Check(info, jc.DeepEquals, jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.model.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer",
			OfferName:              "test-offer",
			ApplicationDescription: "A pretty popular blog engine",
			Endpoints: []jujuparams.RemoteEndpoint{{
				Name:      "url",
				Role:      "provider",
				Interface: "http",
				Limit:     0,
			}},
			Users: []jujuparams.OfferUserDetails{{
				UserName: "test-user@external",
				Access:   "admin",
			}, {
				UserName:    "admin",
				DisplayName: "admin",
				Access:      "admin",
			}, {
				UserName:    "everyone@external",
				DisplayName: "",
				Access:      "read",
			}},
		},
		ControllerInfo: &jujuparams.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(s.ControllerConfig.ControllerUUID()).String(),
			Addrs:         s.APIInfo(c).Addrs,
			CACert:        s.APIInfo(c).CACert,
		},
	})
}

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetailsNotFound(c *gc.C) {
	var info jujuparams.ConsumeOfferDetails
	info.Offer = &jujuparams.ApplicationOfferDetails{
		OfferURL: "test-user@external/test-model.test-offer",
	}
	err := s.conn.GetApplicationOfferConsumeDetails(context.Background(), params.User("test-user"), &info, bakery.Version2)
	c.Check(err, gc.ErrorMatches, `api error: application offer "test-user@external/test-model.test-offer" not found`)
}
