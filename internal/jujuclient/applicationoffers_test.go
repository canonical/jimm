// Copyright 2024 Canonical.

package jujuclient_test

import (
	"context"
	"sort"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/juju/juju/core/crossmodel"
	jujuparams "github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing/factory"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/canonical/jimm/v3/internal/testutils/jimmtest"
)

type applicationoffersSuite struct {
	jujuclientSuite

	modelInfo jujuparams.ModelInfo
}

var _ = gc.Suite(&applicationoffersSuite{})

func (s *applicationoffersSuite) SetUpTest(c *gc.C) {
	s.jujuclientSuite.SetUpTest(c)

	ctx := context.Background()
	err := s.API.CreateModel(ctx, &jujuparams.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: names.NewUserTag("test-user@canonical.com").String(),
	}, &s.modelInfo)
	c.Assert(err, gc.Equals, nil)
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

	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}

	ctx := context.Background()
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		})
	c.Assert(err, gc.Equals, nil)

	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)
}

func (s *applicationoffersSuite) TestOfferError(c *gc.C) {
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}

	err := s.API.Offer(
		context.Background(),
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				"url": "url",
			},
		},
	)
	c.Check(err, gc.ErrorMatches, `getting offered application test-app: application "test-app" not found`)
}

func (s *applicationoffersSuite) TestListApplicationOffersError(c *gc.C) {
	_, err := s.API.ListApplicationOffers(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, `at least one offer filter is required`)
}

func (s *applicationoffersSuite) TestListApplicationOffersNoOffers(c *gc.C) {
	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.ListApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = offerURL.String()
	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetailsV5{info})
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName:       owner.Id(),
		ModelName:       s.modelInfo.Name,
		ApplicationName: "no-such-app",
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.HasLen, 0)
}

func (s *applicationoffersSuite) TestFindApplicationOffersError(c *gc.C) {
	_, err := s.API.FindApplicationOffers(context.Background(), nil)
	c.Assert(err, gc.ErrorMatches, `at least one offer filter is required`)
}

func (s *applicationoffersSuite) TestFindApplicationOffersNoOffers(c *gc.C) {
	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.FindApplicationOffers(context.Background(), []jujuparams.OfferFilter{{
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = offerURL.String()
	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Check(offers, gc.DeepEquals, []jujuparams.ApplicationOfferAdminDetailsV5{info})
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.FindApplicationOffers(ctx, []jujuparams.OfferFilter{{
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = offerURL.String()
	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `ch:amd64/quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               offerURL.String(),
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

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = "test-user@canonical.com/test-model.test-offer"
	err := s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.ErrorMatches, `application offer "test-user@canonical.com/test-model.test-offer" not found`)
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GrantApplicationOfferAccess(ctx, offerURL.String(), names.NewUserTag("test-user-2@canonical.com"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = offerURL.String()
	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `ch:amd64/quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               offerURL.String(),
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
				UserName: "test-user-2@canonical.com",
				Access:   string(jujuparams.OfferConsumeAccess),
			}},
		},
		ApplicationName: "test-app",
	})
}

func (s *applicationoffersSuite) TestGrantApplicationOfferAccessNotFound(c *gc.C) {
	ctx := context.Background()
	offerURL := "test-user@canonical.com/test-model.test-offer"

	err := s.API.GrantApplicationOfferAccess(ctx, offerURL, names.NewUserTag("test-user-2@canonical.com"), jujuparams.OfferConsumeAccess)
	c.Check(err, gc.ErrorMatches, `offer "test-offer" not found`)
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GrantApplicationOfferAccess(ctx, offerURL.String(), names.NewUserTag("test-user-2@canonical.com"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ApplicationOfferAdminDetailsV5
	info.OfferURL = offerURL.String()
	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)

	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `ch:amd64/quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               "test-user@canonical.com/test-model.test-offer",
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
				UserName: "test-user-2@canonical.com",
				Access:   string(jujuparams.OfferConsumeAccess),
			}},
		},
		ApplicationName: "test-app",
	})

	err = s.API.RevokeApplicationOfferAccess(ctx, offerURL.String(), names.NewUserTag("test-user-2@canonical.com"), jujuparams.OfferConsumeAccess)
	c.Assert(err, gc.Equals, nil)

	err = s.API.GetApplicationOffer(ctx, &info)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.OfferUUID, gc.Not(gc.Equals), "")
	info.OfferUUID = ""
	sort.Slice(info.Users, func(i, j int) bool {
		return info.Users[i].UserName < info.Users[j].UserName
	})
	c.Check(info.CharmURL, gc.Matches, `ch:amd64/quantal/wordpress-[0-9]*`)
	info.CharmURL = ""
	c.Check(info, jc.DeepEquals, jujuparams.ApplicationOfferAdminDetailsV5{
		ApplicationOfferDetailsV5: jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               offerURL.String(),
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
				UserName: "test-user-2@canonical.com",
				Access:   string(jujuparams.OfferReadAccess),
			}},
		},
		ApplicationName: "test-app",
	})
}

func (s *applicationoffersSuite) TestRevokeApplicationOfferAccessNotFound(c *gc.C) {
	ctx := context.Background()
	offerURL := "test-user@canonical.com/test-model.test-offer"

	err := s.API.RevokeApplicationOfferAccess(ctx, offerURL, names.NewUserTag("test-user-2@canonical.com"), jujuparams.OfferConsumeAccess)
	c.Check(err, gc.ErrorMatches, `offer "test-offer" not found`)
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	owner, err := names.ParseUserTag(s.modelInfo.OwnerTag)
	c.Assert(err, gc.Equals, nil)
	offers, err := s.API.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
		OwnerName: owner.Id(),
		ModelName: s.modelInfo.Name,
	}})
	c.Assert(err, gc.Equals, nil)
	c.Assert(offers, gc.HasLen, 1)

	err = s.API.DestroyApplicationOffer(ctx, offerURL.String(), false)
	c.Assert(err, gc.Equals, nil)

	offers, err = s.API.ListApplicationOffers(ctx, []jujuparams.OfferFilter{{
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
	offerURL := crossmodel.OfferURL{
		User:            "test-user@canonical.com",
		ModelName:       s.modelInfo.Name,
		ApplicationName: "test-offer",
	}
	err = s.API.Offer(
		ctx,
		offerURL,
		jujuparams.AddApplicationOffer{
			ModelTag:        names.NewModelTag(s.modelInfo.UUID).String(),
			OfferName:       "test-offer",
			ApplicationName: "test-app",
			Endpoints: map[string]string{
				ep.Name: ep.Name,
			},
		},
	)
	c.Assert(err, gc.Equals, nil)

	var info jujuparams.ConsumeOfferDetails
	info.Offer = &jujuparams.ApplicationOfferDetailsV5{
		OfferURL: offerURL.String(),
	}
	err = s.API.GetApplicationOfferConsumeDetails(ctx, names.NewUserTag("admin"), &info, bakery.Version2)
	c.Assert(err, gc.Equals, nil)
	c.Check(info.Offer.OfferUUID, gc.Not(gc.Equals), "")
	info.Offer.OfferUUID = ""
	c.Check(info.Macaroon, gc.Not(gc.IsNil))
	info.Macaroon = nil
	lessF := func(a, b jujuparams.OfferUserDetails) bool {
		return a.UserName < b.UserName
	}
	c.Check(info, jimmtest.CmpEquals(cmpopts.SortSlices(lessF)), jujuparams.ConsumeOfferDetails{
		Offer: &jujuparams.ApplicationOfferDetailsV5{
			SourceModelTag:         names.NewModelTag(s.modelInfo.UUID).String(),
			OfferURL:               "test-user@canonical.com/test-model.test-offer",
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
	info.Offer = &jujuparams.ApplicationOfferDetailsV5{
		OfferURL: "test-user@canonical.com/test-model.test-offer",
	}
	err := s.API.GetApplicationOfferConsumeDetails(context.Background(), names.NewUserTag("test-user@canonical.com"), &info, bakery.Version2)
	c.Check(err, gc.ErrorMatches, `application offer "test-user@canonical.com/test-model.test-offer" not found`)
}
