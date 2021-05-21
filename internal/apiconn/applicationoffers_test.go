// Copyright 2020 Canonical Ltd.

package apiconn_test

import (
	"context"
	"sort"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/api"
	jujuparams "github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/apiconn"
	"github.com/CanonicalLtd/jimm/internal/conv"
	"github.com/CanonicalLtd/jimm/internal/jemtest"
	"github.com/CanonicalLtd/jimm/params"
)

type applicationoffersSuite struct {
	jemtest.JujuConnSuite

	cache     *apiconn.Cache
	conn      *apiconn.Conn
	modelInfo jujuparams.ModelInfo
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

	err = s.conn.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: conv.ToUserTag("test-user").String(),
	}, &s.modelInfo)
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
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	// Note that the behaviour of offer changed at some point in the
	// 2.9 development such that an existing offer is updated, rather
	// than producing an error.
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationoffersSuite) TestOfferError(c *gc.C) {
	err := s.conn.Offer(context.Background(), jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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
	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.ListApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestListApplicationOffersMatching(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{info})
}

func (s *applicationoffersSuite) TestListApplicationOffersNoMatch(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName:       owner.Id(),
		ModelName:       s.modelInfo.Name,
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
	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.FindApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestFindApplicationOffersMatching(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetails{info})
}

func (s *applicationoffersSuite) TestFindApplicationOffersNoMatch(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName:       owner.Id(),
		ModelName:       s.modelInfo.Name,
		ApplicationName: "no-such-app",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestGetApplicationOffer(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
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
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
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
	c.Check(err, gc.ErrorMatches, `api error: offer "test-offer" not found`)
}

func (s *applicationoffersSuite) TestRevokeApplicationOfferAccess(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
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
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
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
	c.Check(err, gc.ErrorMatches, `api error: offer "test-offer" not found`)
}

func (s *applicationoffersSuite) TestDestroyApplicationOffer(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer",
		ApplicationName: "test-app",
		Endpoints: map[string]string{
			ep.Name: ep.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 1)

	offerURL := "test-user@external/test-model.test-offer"
	err = s.conn.DestroyApplicationOffer(ctx, offerURL, false)
	c.Assert(err, gc.Equals, nil)

	offers, err = s.conn.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestGetApplicationOfferConsumeDetails(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
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
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
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
	lessF := func(a, b jujuparams.OfferUserDetails) bool {
		return a.UserName < b.UserName
	}
	c.Check(info, jemtest.CmpEquals(cmpopts.SortSlices(lessF)), jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
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
				Access:      "admin",
			}, {
				UserName:    "everyone@external",
				DisplayName: "",
				Access:      "read",
			}, {
				UserName: "test-user@external",
				Access:   "admin",
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

func (s *applicationoffersSuite) TestGetApplicationOffers(c *gc.C) {
	modelState, err := s.StatePool.Get(s.modelInfo.UUID)
	c.Assert(err, gc.Equals, nil)
	defer modelState.Release()
	f := factory.NewFactory(modelState.State, s.StatePool)
	app1 := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app1",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app1,
	})
	ep1, err := app1.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	app2 := f.MakeApplication(c, &factory.ApplicationParams{
		Name: "test-app2",
		Charm: f.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	f.MakeUnit(c, &factory.UnitParams{
		Application: app2,
	})
	ep2, err := app2.Endpoint("url")
	c.Assert(err, gc.Equals, nil)

	ctx := context.Background()
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer1",
		ApplicationName: "test-app1",
		Endpoints: map[string]string{
			ep1.Name: ep1.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)
	err = s.conn.Offer(ctx, jujuparams.AddApplicationOffer{
		ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
		OfferName:       "test-offer2",
		ApplicationName: "test-app2",
		Endpoints: map[string]string{
			ep2.Name: ep2.Name,
		},
	})
	c.Assert(err, gc.Equals, nil)

	var info1 jujuparams.ApplicationOfferAdminDetails
	info1.OfferURL = "test-user@external/test-model.test-offer1"
	var info2 jujuparams.ApplicationOfferAdminDetails
	info2.OfferURL = "test-user@external/test-model.test-offer2"
	err = s.conn.GetApplicationOffers(ctx, []*jujuparams.ApplicationOfferAdminDetails{&info1, &info2})
	c.Assert(err, gc.Equals, nil)

	c.Check(info1.OfferUUID, gc.Not(gc.Equals), "")
	info1.OfferUUID = ""
	sort.Slice(info1.Users, func(i, j int) bool {
		return info1.Users[i].UserName < info1.Users[j].UserName
	})
	c.Check(info1.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info1.CharmURL = ""
	c.Check(info1, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer1",
			OfferName:              "test-offer1",
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
		ApplicationName: "test-app1",
	})

	c.Check(info2.OfferUUID, gc.Not(gc.Equals), "")
	info2.OfferUUID = ""
	sort.Slice(info2.Users, func(i, j int) bool {
		return info2.Users[i].UserName < info2.Users[j].UserName
	})
	c.Check(info2.CharmURL, gc.Matches, `cs:quantal/wordpress-[0-9]*`)
	info2.CharmURL = ""
	c.Check(info2, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetails{
		ApplicationOfferDetails: jujuparams.ApplicationOfferDetails{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               "test-user@external/test-model.test-offer2",
			OfferName:              "test-offer2",
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
		ApplicationName: "test-app2",
	})
}
